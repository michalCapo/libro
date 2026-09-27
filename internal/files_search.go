package libro

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

type fileMatch struct {
	Context   string `json:"context,omitempty"`
	StartLine int    `json:"startLine,omitempty"`
	Parents   int    `json:"parents,omitempty"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Text      string `json:"text"`
}

// Ripgrep applies project ignore rules and never follows directory symlinks.
// Bound both elapsed time and output; all arguments are passed without a shell.
func searchProjectFiles(root, query string, includeIgnored, index bool) (fileResult, error) {
	result := fileResult{}
	if root == "" {
		return result, fmt.Errorf("open a project to search its files")
	}
	if len(query) > 4096 {
		return result, fmt.Errorf("search text is too long (maximum 4096 bytes)")
	}
	if !index && strings.TrimSpace(query) == "" {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"--no-config", "--glob", "!.git", "--glob", "!.git/**"}
	if includeIgnored {
		args = append(args, "--hidden", "--no-ignore")
	}
	if index {
		args = append(args, "--files", "--null")
	} else {
		args = append(args, "--json", "--multiline", "--fixed-strings", "--smart-case", "--max-filesize", "1M", "--", query)
	}
	args = append(args, ".")
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = root
	cmd.WaitDelay = time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	if err = cmd.Start(); err != nil {
		return result, fmt.Errorf("project search requires ripgrep (rg): %w", err)
	}
	output := &io.LimitedReader{R: stdout, N: 16 << 20}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	if index {
		scanner.Split(splitFilePath)
	}
	for scanner.Scan() {
		if index {
			path := strings.TrimPrefix(scanner.Text(), "./")
			if filepath.IsLocal(path) {
				result.Entries = append(result.Entries, fileEntry{Name: filepath.Base(path), Path: filepath.ToSlash(filepath.Clean(path))})
			}
			if len(result.Entries) >= 20000 {
				result.Truncated = true
				cancel()
				break
			}
			continue
		}
		var event struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				Line    int `json:"line_number"`
				Matches []struct {
					Start int `json:"start"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "match" || len(event.Data.Matches) == 0 {
			continue
		}
		path := strings.TrimPrefix(event.Data.Path.Text, "./")
		if !filepath.IsLocal(path) {
			continue
		}
		column := event.Data.Matches[0].Start
		if column > len(event.Data.Lines.Text) {
			continue
		}
		text := strings.TrimRight(event.Data.Lines.Text, "\r\n")
		if len(text) > 2000 {
			text = string([]rune(text)[:min(500, len([]rune(text)))]) + "…"
		}
		result.Matches = append(result.Matches, fileMatch{Path: filepath.ToSlash(filepath.Clean(path)), Line: event.Data.Line, Column: len(utf16.Encode([]rune(event.Data.Lines.Text[:column]))), Text: text})
		if len(result.Matches) >= 500 {
			result.Truncated = true
			cancel()
			break
		}
	}
	scanErr := scanner.Err()
	if output.N == 0 {
		result.Truncated = true
		cancel()
	}
	if scanErr != nil {
		cancel()
	}
	err = cmd.Wait()
	if result.Truncated {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("search timed out; narrow your search")
	}
	if scanErr != nil {
		return result, scanErr
	}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return result, nil
		}
		return result, fmt.Errorf("could not search project: %w", err)
	}
	return result, nil
}
func splitFilePath(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == 0 {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

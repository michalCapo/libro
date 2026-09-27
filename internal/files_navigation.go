package libro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/sourcegraph/jsonrpc2"
)

var fileNavigationMethods = map[string]string{
	"definition": "textDocument/definition", "references": "textDocument/references",
	"declaration": "textDocument/declaration", "implementation": "textDocument/implementation",
	"typeDefinition": "textDocument/typeDefinition", "restart": "restart",
}

type navigationLanguage struct {
	command  string
	args     []string
	language string
	markers  []string
}

func fileNavigationLanguage(path string) (navigationLanguage, error) {
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".go":
		return navigationLanguage{"gopls", nil, "go", []string{"go.work", "go.mod"}}, nil
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		language := "javascript"
		if strings.Contains(extension, "t") {
			language = "typescript"
		}
		if strings.HasSuffix(extension, "x") {
			language += "react"
		}
		return navigationLanguage{"typescript-language-server", []string{"--stdio"}, language, []string{"tsconfig.json", "jsconfig.json", "package.json"}}, nil
	case ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".m", ".mm":
		language := "cpp"
		switch strings.ToLower(filepath.Ext(path)) {
		case ".c", ".h":
			language = "c"
		case ".m":
			language = "objective-c"
		case ".mm":
			language = "objective-cpp"
		}
		return navigationLanguage{"clangd", []string{"--background-index"}, language, []string{"compile_commands.json", "compile_flags.txt", "CMakeLists.txt", "Makefile"}}, nil
	default:
		return navigationLanguage{}, fmt.Errorf("code navigation supports Go, JavaScript/TypeScript and C/C++/Objective-C files")
	}
}

func navigationRoot(root, file string, markers []string) string {
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		if dir == root || filepath.Dir(dir) == dir {
			break
		}
	}
	return filepath.Dir(file)
}

func fileNavigationURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

type navigationPipes struct {
	io.ReadCloser
	writer io.WriteCloser
}

func (p navigationPipes) Write(data []byte) (int, error) { return p.writer.Write(data) }
func (p navigationPipes) Close() error                   { _ = p.writer.Close(); return p.ReadCloser.Close() }

func navigationServerRequest(_ context.Context, _ *jsonrpc2.Conn, request *jsonrpc2.Request) (any, error) {
	switch request.Method {
	case "workspace/configuration":
		var params struct {
			Items []json.RawMessage `json:"items"`
		}
		if request.Params != nil {
			_ = json.Unmarshal(*request.Params, &params)
		}
		return make([]any, len(params.Items)), nil
	case "workspace/applyEdit":
		return map[string]any{"applied": false, "failureReason": "Files is read only"}, nil
	case "client/registerCapability", "client/unregisterCapability", "window/workDoneProgress/create":
		return nil, nil
	}
	if request.Notif {
		return nil, nil
	}
	return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: "Unsupported client request"}
}

// Servers retain their project analysis between navigation requests.
func startNavigationServer(ctx context.Context, language navigationLanguage, root string) (*navigationServer, error) {
	executable, err := exec.LookPath(language.command)
	if err != nil {
		return nil, fmt.Errorf("install %s and make it available on PATH to navigate this language", language.command)
	}
	processContext, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(processContext, executable, language.args...)
	prepareNavigationProcess(cmd)
	cmd.Dir = root
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		cancel()
		return nil, err
	}
	connection := jsonrpc2.NewConn(processContext, jsonrpc2.NewBufferedStream(navigationPipes{output, input}, jsonrpc2.VSCodeObjectCodec{}), jsonrpc2.HandlerWithError(navigationServerRequest).SuppressErrClosed())
	server := &navigationServer{connection: connection, cmd: cmd, cancel: cancel, documents: make(map[string]navigationDocument)}
	success := false
	defer func() {
		if !success {
			server.close()
		}
	}()
	var initialized struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	params := map[string]any{
		"processId": os.Getpid(), "rootUri": fileNavigationURI(root),
		"clientInfo":   map[string]string{"name": "Libro Files"},
		"capabilities": map[string]any{"general": map[string]any{"positionEncodings": []string{"utf-16"}}, "textDocument": map[string]any{"definition": map[string]bool{"linkSupport": true}, "declaration": map[string]bool{"linkSupport": true}, "typeDefinition": map[string]bool{"linkSupport": true}, "implementation": map[string]bool{"linkSupport": true}}},
	}
	if language.command == "typescript-language-server" {
		// A fresh server otherwise routes navigation to its syntax-only worker
		// while the project loads, which cannot resolve imported symbols.
		params["initializationOptions"] = map[string]any{"tsserver": map[string]string{"useSyntaxServer": "never"}}
	}
	if err = connection.Call(ctx, "initialize", params, &initialized); err != nil {
		return nil, fmt.Errorf("%s initialization: %w", language.command, err)
	}
	if encoding := initialized.Capabilities["positionEncoding"]; len(encoding) > 0 && string(encoding) != `"utf-16"` {
		return nil, fmt.Errorf("%s did not accept UTF-16 positions", language.command)
	}
	if err = connection.Notify(ctx, "initialized", struct{}{}); err != nil {
		return nil, err
	}
	server.capabilities = initialized.Capabilities
	success = true
	return server, nil
}

type navigationPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type navigationRange struct {
	Start navigationPosition `json:"start"`
}
type navigationLocation struct {
	URI                  string          `json:"uri"`
	Range                navigationRange `json:"range"`
	TargetURI            string          `json:"targetUri"`
	TargetSelectionRange navigationRange `json:"targetSelectionRange"`
}

func navigationLocations(project string, raw json.RawMessage) (fileResult, error) {
	result := fileResult{}
	if len(raw) == 0 || string(raw) == "null" {
		return result, nil
	}
	var locations []navigationLocation
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &locations); err != nil {
			return result, err
		}
	} else {
		var location navigationLocation
		if err := json.Unmarshal(raw, &location); err != nil {
			return result, err
		}
		locations = append(locations, location)
	}
	seen := map[string]bool{}
	sources := map[string][]string{}
	for _, location := range locations {
		uri, position := location.URI, location.Range.Start
		if location.TargetURI != "" {
			uri, position = location.TargetURI, location.TargetSelectionRange.Start
		}
		target, err := url.Parse(uri)
		if err != nil || target.Scheme != "file" || target.Host != "" && target.Host != "localhost" || position.Line < 0 || position.Character < 0 {
			continue
		}
		path := target.Path
		if runtime.GOOS == "windows" {
			path = strings.TrimPrefix(path, "/")
		}
		path, err = filepath.EvalSymlinks(filepath.FromSlash(path))
		if err != nil {
			continue
		}
		// Dependencies may live outside the workspace. Use the existing parent-root
		// browsing mechanism, so every preview still passes the same local-path checks.
		root, parents := project, 0
		var relative string
		for {
			resolved, resolveErr := filepath.EvalSymlinks(root)
			if resolveErr != nil {
				break
			}
			relative, err = filepath.Rel(resolved, path)
			if err == nil && filepath.IsLocal(relative) {
				break
			}
			if filepath.Dir(root) == root {
				break
			}
			root = filepath.Dir(root)
			parents++
		}
		if err != nil || !filepath.IsLocal(relative) {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", path, position.Line, position.Character)
		if seen[key] {
			continue
		}
		seen[key] = true
		match := fileMatch{Path: filepath.ToSlash(relative), Parents: parents, Line: position.Line + 1, Column: position.Character}
		lines, loaded := sources[path]
		if !loaded {
			source, readErr := readProjectFile(root, relative)
			if readErr == nil && source.MIME == "" {
				lines = strings.Split(source.Text, "\n")
			}
			sources[path] = lines
		}
		if position.Line < len(lines) {
			start, end := max(0, position.Line-3), min(len(lines), position.Line+4)
			match.StartLine = start + 1
			match.Context = strings.Join(lines[start:end], "\n")
		}
		result.Matches = append(result.Matches, match)
		if len(result.Matches) >= 200 {
			result.Truncated = len(locations) > 200
			break
		}
	}
	return result, nil
}

func navigateProjectFile(project, path, kind, version string, parents, line, column int) (fileResult, error) {
	result := fileResult{}
	method := fileNavigationMethods[kind]
	if method == "" {
		return result, fmt.Errorf("unknown navigation action")
	}
	if project == "" {
		return result, fmt.Errorf("open a project to navigate code")
	}
	project, err := filepath.Abs(project)
	if err != nil {
		return result, err
	}
	browseProject := project
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return result, err
	}
	root := filesRoot(project, parents)
	file, err := projectFileToOpen(root, path)
	if err != nil {
		return result, err
	}
	source, err := readProjectFile(root, path)
	if err != nil {
		return result, err
	}
	if source.MIME != "" {
		return result, fmt.Errorf("code navigation requires a source file")
	}
	if version != "" && source.Version != version {
		return result, fmt.Errorf("file changed on disk; wait for the preview to refresh and try again")
	}
	lines := strings.Split(source.Text, "\n")
	if line < 1 || line > len(lines) || column < 0 || column > len(utf16.Encode([]rune(lines[line-1]))) {
		return result, fmt.Errorf("invalid source position")
	}
	language, err := fileNavigationLanguage(path)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := requestFileNavigation(ctx, language, navigationRoot(root, file, language.markers), file, source.Text, method, line, column)
	if ctx.Err() != nil {
		return result, fmt.Errorf("code navigation timed out; check the language server and project configuration")
	}
	if err != nil {
		return result, err
	}
	return navigationLocations(browseProject, raw)
}

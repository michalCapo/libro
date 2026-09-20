package libro

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

var toolIconMu sync.Mutex

// Cache both downloaded icons and failed lookups. Missing icons are retried daily.
func toolIcon(p Plugin) string {
	if !configurableTool(p) || p.Removed {
		return ""
	}
	source := string(p.Type) + ":" + p.Command + p.URL
	dir, err := libroDataDir()
	if err != nil {
		return ""
	}
	file := filepath.Join(dir, "tool-icons", fmt.Sprintf("%x", sha256.Sum256([]byte(source))))
	toolIconMu.Lock()
	defer toolIconMu.Unlock()
	if data, err := os.ReadFile(file); err == nil {
		if len(data) > 0 {
			return string(data)
		}
		if info, err := os.Stat(file); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
			return ""
		}
	}
	var icon string
	if p.Type == AppTypeURL {
		icon = websiteToolIcon(p.URL)
	} else {
		icon = localToolIcon(p.Command)
		if icon == "" && pluginIDPattern.MatchString(extractBaseCmd(p.Command)) {
			address := ""
			if known := lookupTermIcon(p.Command); known != nil {
				address = known.URL
			} else {
				address = discoverTermIconURL(p.Command)
			}
			if address != "" {
				icon = downloadToolIcon(address)
			}
		}
	}
	if os.MkdirAll(filepath.Dir(file), 0700) == nil {
		_ = os.WriteFile(file, []byte(icon), 0600)
	}
	return icon
}

func downloadToolIcon(address string) string {
	client := &http.Client{Timeout: 4 * time.Second}
	res, err := client.Get(address)
	if err != nil {
		return ""
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 512*1024+1))
	if err != nil {
		return ""
	}
	return iconData(data)
}

func iconData(data []byte) string {
	if len(data) == 0 || len(data) > 512*1024 {
		return ""
	}
	mime := http.DetectContentType(data)
	if strings.Contains(string(data[:min(len(data), 512)]), "<svg") {
		mime = "image/svg+xml"
	}
	if !strings.HasPrefix(mime, "image/") {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func websiteToolIcon(address string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	fetch := func(address string, limit int64) ([]byte, *url.URL) {
		u, err := url.Parse(address)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return nil, nil
		}
		req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
		if err != nil {
			return nil, nil
		}
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.User != nil {
				return fmt.Errorf("invalid icon redirect")
			}
			return nil
		}}
		res, err := client.Do(req)
		if err != nil {
			return nil, nil
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			return nil, nil
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
		if err != nil || int64(len(data)) > limit {
			return nil, nil
		}
		return data, res.Request.URL
	}
	// Strip credentials, query strings and fragments from discovery requests.
	base, err := url.Parse(address)
	if err != nil {
		return ""
	}
	base.User = nil
	base.RawQuery = ""
	base.Fragment = ""
	page, final := fetch(base.String(), 1024*1024)
	if final != nil {
		base = final
	}
	var candidates []string
	tokenizer := html.NewTokenizer(strings.NewReader(string(page)))
	for tokenizer.Next() != html.ErrorToken {
		token := tokenizer.Token()
		if token.Data != "link" {
			continue
		}
		var rel, href string
		for _, attr := range token.Attr {
			if attr.Key == "rel" {
				rel = strings.ToLower(attr.Val)
			}
			if attr.Key == "href" {
				href = attr.Val
			}
		}
		for value := range strings.FieldsSeq(rel) {
			if value == "icon" || value == "apple-touch-icon" {
				if ref, err := url.Parse(href); err == nil && href != "" {
					candidates = append(candidates, base.ResolveReference(ref).String())
				}
				break
			}
		}
		if len(candidates) >= 5 {
			break
		}
	}
	candidates = append(candidates, base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String())
	for _, candidate := range candidates {
		data, _ := fetch(candidate, 512*1024)
		if icon := iconData(data); icon != "" {
			return icon
		}
	}
	return ""
}

// CLI tools often ship an icon with their desktop integration, even when they
// run in a terminal. Never execute the command to discover its icon.
func localToolIcon(command string) string {
	name := filepath.Base(extractBaseCmd(command))
	if name == "." || name == "" {
		return ""
	}
	roots := []string{"/usr/local/share", "/usr/share"}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append([]string{filepath.Join(home, ".local/share")}, roots...)
	}
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		roots = append([]string{data}, roots...)
	}
	for _, root := range roots {
		for _, pattern := range []string{"icons/hicolor/*/apps/" + name + ".*", "pixmaps/" + name + ".*"} {
			files, _ := filepath.Glob(filepath.Join(root, pattern))
			for _, file := range files {
				f, err := os.Open(file)
				if err != nil {
					continue
				}
				data, err := io.ReadAll(io.LimitReader(f, 512*1024+1))
				_ = f.Close()
				if err == nil {
					if icon := iconData(data); icon != "" {
						return icon
					}
				}
			}
		}
	}
	return ""
}

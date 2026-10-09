package components

import (
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var terminalURLPattern = regexp.MustCompile(`(?i)https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]|\[::\]):[^\s\x00-\x20<>"']+`)
var terminalANSIPattern = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

// Keep startup URLs after the bounded terminal log has rolled over.
type terminalURLs struct {
	urls []string
	tail string
}

func (u *terminalURLs) append(data []byte) {
	text := u.tail + string(data)
	start := 0
	for i := range len(text) {
		if text[i] == '\n' || text[i] == '\r' {
			u.urls = collectTerminalURLs(u.urls, text[start:i])
			start = i + 1
		}
	}
	u.tail = text[start:]
	if len(u.tail) > 4096 {
		u.tail = u.tail[len(u.tail)-4096:]
	}
}

func collectTerminalURLs(urls []string, text string) []string {
	if !strings.Contains(strings.ToLower(text), "http") {
		return urls
	}
	text = terminalANSIPattern.ReplaceAllString(text, "")
	for _, address := range terminalURLPattern.FindAllString(text, -1) {
		parsed, err := url.Parse(strings.TrimRight(address, ".,;)]}"))
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		// Wildcard bind addresses are opened through localhost. Keep advertised paths.
		parsed.Host = "localhost:" + strconv.Itoa(port)
		// One advertised address per origin keeps request logs out of the picker.
		if !slices.ContainsFunc(urls, func(existing string) bool {
			current, err := url.Parse(existing)
			return err == nil && current.Scheme == parsed.Scheme && current.Host == parsed.Host
		}) {
			urls = append(urls, parsed.String())
		}
	}
	return urls
}

// LocalURLs returns local HTTP addresses printed by this terminal's current launch.
func (tm *TerminalManager) LocalURLs(id string) []string {
	tm.mu.Lock()
	log := tm.logs[id]
	tm.mu.Unlock()
	if log == nil {
		return nil
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	return collectTerminalURLs(slices.Clone(log.urls.urls), log.urls.tail)
}

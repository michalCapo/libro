package components

import "sync"

const terminalLogLimit = 4 * 1024 * 1024

type terminalLog struct {
	managed   *managedTerminal
	mu        sync.Mutex
	data      []byte
	truncated bool
	urls      terminalURLs
}

func (l *terminalLog) append(data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.urls.append(data)
	if len(data) >= terminalLogLimit {
		l.truncated = l.truncated || len(l.data)+len(data) > terminalLogLimit
		l.data = append(l.data[:0], data[len(data)-terminalLogLimit:]...)
		return
	}
	if excess := len(l.data) + len(data) - terminalLogLimit; excess > 0 {
		copy(l.data, l.data[excess:])
		l.data = l.data[:len(l.data)-excess]
		l.truncated = true
	}
	l.data = append(l.data, data...)
}

// Logs returns recent combined PTY output, retained until Stop or a new launch.
func (tm *TerminalManager) Logs(id string) (string, bool) {
	tm.mu.Lock()
	log := tm.logs[id]
	tm.mu.Unlock()
	if log == nil {
		return "", false
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	return string(log.data), log.truncated
}

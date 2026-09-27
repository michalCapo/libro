package libro

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

type navigationDocument struct {
	text    string
	version int
}
type navigationServer struct {
	connection   *jsonrpc2.Conn
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	capabilities map[string]json.RawMessage
	documents    map[string]navigationDocument
}

func (s *navigationServer) close() {
	s.cancel()
	_ = s.connection.Close()
	_ = s.cmd.Wait()
}

type navigationSession struct {
	gate     chan struct{}
	server   *navigationServer
	timer    *time.Timer
	lastUsed time.Time
}

var navigationPool = struct {
	sync.Mutex
	sessions map[string]*navigationSession
}{sessions: make(map[string]*navigationSession)}

const navigationIdleTimeout = 10 * time.Minute

// Acquire without holding the pool lock while waiting on another request.
func acquireNavigationSession(ctx context.Context, key string) (*navigationSession, error) {
	for {
		navigationPool.Lock()
		session := navigationPool.sessions[key]
		if session == nil {
			session = &navigationSession{gate: make(chan struct{}, 1)}
			navigationPool.sessions[key] = session
		}
		navigationPool.Unlock()
		select {
		case session.gate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		navigationPool.Lock()
		current := navigationPool.sessions[key] == session
		if current && session.timer != nil {
			session.timer.Stop()
		}
		navigationPool.Unlock()
		if current {
			return session, nil
		}
		<-session.gate
	}
}
func releaseNavigationSession(key string, session *navigationSession) {
	navigationPool.Lock()
	session.lastUsed = time.Now()
	session.timer = time.AfterFunc(navigationIdleTimeout, func() { retireNavigationSession(key, session, true) })
	navigationPool.Unlock()
	<-session.gate
}
func retireNavigationSession(key string, session *navigationSession, idle bool) {
	session.gate <- struct{}{}
	defer func() { <-session.gate }()
	navigationPool.Lock()
	if navigationPool.sessions[key] != session || (idle && time.Since(session.lastUsed) < navigationIdleTimeout) {
		navigationPool.Unlock()
		return
	}
	delete(navigationPool.sessions, key)
	if session.timer != nil {
		session.timer.Stop()
	}
	navigationPool.Unlock()
	if session.server != nil {
		session.server.close()
		session.server = nil
	}
}
func closeNavigationServers() {
	navigationPool.Lock()
	sessions := maps.Clone(navigationPool.sessions)
	navigationPool.Unlock()
	for key, session := range sessions {
		retireNavigationSession(key, session, false)
	}
}

func requestFileNavigation(ctx context.Context, language navigationLanguage, root, file, text, method string, line, column int) (json.RawMessage, error) {
	key := root + "\x00" + language.command + "\x00" + strings.Join(language.args, "\x00")
	session, err := acquireNavigationSession(ctx, key)
	if err != nil {
		return nil, err
	}
	defer releaseNavigationSession(key, session)
	if session.server != nil {
		disconnected := false
		select {
		case <-session.server.connection.DisconnectNotify():
			disconnected = true
		default:
		}
		if method == "restart" || disconnected {
			session.server.close()
			session.server = nil
		}
	}
	if session.server == nil {
		session.server, err = startNavigationServer(ctx, language, root)
		if err != nil {
			return nil, err
		}
	}
	server := session.server
	result, err := server.navigate(ctx, language, file, text, method, line, column)
	disconnected := false
	select {
	case <-server.connection.DisconnectNotify():
		disconnected = true
	default:
	}
	if err != nil && (ctx.Err() != nil || disconnected) {
		server.close()
		session.server = nil
	}
	return result, err
}

func (s *navigationServer) navigate(ctx context.Context, language navigationLanguage, file, text, method string, line, column int) (json.RawMessage, error) {
	// Refresh previously opened buffers so disk edits never leave stale overlays.
	for path, document := range s.documents {
		if path == file {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			if err = s.connection.Notify(ctx, "textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": fileNavigationURI(path)}}); err != nil {
				return nil, err
			}
			delete(s.documents, path)
		} else if string(content) != document.text {
			if err = s.change(ctx, path, string(content), document); err != nil {
				return nil, err
			}
		}
	}
	uri := fileNavigationURI(file)
	if document, ok := s.documents[file]; ok {
		if document.text != text {
			if err := s.change(ctx, file, text, document); err != nil {
				return nil, err
			}
		}
	} else {
		// Bound retained read-only buffers during long browsing sessions.
		if len(s.documents) >= 32 {
			for path := range s.documents {
				if err := s.connection.Notify(ctx, "textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": fileNavigationURI(path)}}); err != nil {
					return nil, err
				}
				delete(s.documents, path)
				break
			}
		}
		if err := s.connection.Notify(ctx, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": language.language, "version": 1, "text": text}}); err != nil {
			return nil, err
		}
		s.documents[file] = navigationDocument{text: text, version: 1}
	}
	if method == "restart" {
		return nil, nil
	}
	if language.command == "typescript-language-server" && method == "textDocument/declaration" {
		method = "textDocument/definition"
	}
	capability := strings.TrimPrefix(method, "textDocument/") + "Provider"
	if value := s.capabilities[capability]; len(value) == 0 || string(value) == "false" || string(value) == "null" {
		return nil, fmt.Errorf("%s does not support %s", language.command, strings.TrimPrefix(method, "textDocument/"))
	}
	request := map[string]any{"textDocument": map[string]string{"uri": uri}, "position": map[string]int{"line": line - 1, "character": column}}
	if method == "textDocument/references" {
		request["context"] = map[string]bool{"includeDeclaration": true}
	}
	var result json.RawMessage
	err := s.connection.Call(ctx, method, request, &result)
	return result, err
}
func (s *navigationServer) change(ctx context.Context, path, text string, document navigationDocument) error {
	document.version++
	if err := s.connection.Notify(ctx, "textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": fileNavigationURI(path), "version": document.version}, "contentChanges": []map[string]string{{"text": text}}}); err != nil {
		return err
	}
	document.text = text
	s.documents[path] = document
	return nil
}

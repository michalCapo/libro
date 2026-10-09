package components

import (
	"encoding/json"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestTerminalLogs(t *testing.T) {
	tm := NewTerminalManager()
	log := &terminalLog{}
	tm.logs["app"] = log
	session := &TerminalSession{log: log, connected: true}
	session.broadcastOutput([]byte("stdout\r\nstderr\r\n"))
	for range 2 {
		if output, truncated := tm.Logs("app"); output != "stdout\r\nstderr\r\n" || truncated {
			t.Fatalf("logs are missing or consumed: %q %v", output, truncated)
		}
	}
	session.broadcastOutput([]byte(strings.Repeat("x", terminalLogLimit)))
	session.broadcastOutput([]byte("end"))
	if output, truncated := tm.Logs("app"); len(output) != terminalLogLimit || !strings.HasSuffix(output, "end") || !truncated {
		t.Fatal("logs must retain a bounded tail and report truncation")
	}
	tm.Stop("app")
	if output, truncated := tm.Logs("app"); output != "" || truncated {
		t.Fatal("stop must clear logs even after the process exits")
	}
}

func terminalReplayFixture(t *testing.T, enabled bool) (*TerminalSession, *httptest.Server) {
	t.Helper()
	tm := NewTerminalManager()
	session := &TerminalSession{ID: "app", log: &terminalLog{}, clients: make(map[*terminalClient]bool)}
	tm.sessions[session.ID] = session
	tm.logs[session.ID] = session.log
	tm.SetReplayOutput(enabled)
	server := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		client := &terminalClient{conn: conn}
		session.addClient(client)
		defer session.removeClient(client)
		_ = client.send(terminalWSMessage{Type: "ready"})
		var message []byte
		_ = websocket.Message.Receive(conn, &message)
	}))
	t.Cleanup(server.Close)
	return session, server
}

func receiveTerminalReplay(t *testing.T, server *httptest.Server) string {
	t.Helper()
	conn, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	var output strings.Builder
	for {
		var frame []byte
		if err := websocket.Message.Receive(conn, &frame); err != nil {
			t.Fatal(err)
		}
		if len(frame) > 0 && frame[0] == terminalBinaryDataFrame {
			output.Write(frame[1:])
			continue
		}
		var message terminalWSMessage
		if err := json.Unmarshal(frame, &message); err != nil {
			t.Fatal(err)
		}
		if message.Type == "ready" {
			return output.String()
		}
	}
}

func TestTerminalReconnectReplay(t *testing.T) {
	session, server := terminalReplayFixture(t, true)
	session.broadcastOutput([]byte("startup\n"))
	if output := receiveTerminalReplay(t, server); output != "startup\n" {
		t.Fatalf("first connection = %q", output)
	}
	session.broadcastOutput([]byte("work while disconnected\n"))
	if output := receiveTerminalReplay(t, server); output != "startup\nwork while disconnected\n" {
		t.Fatalf("reconnected browser = %q", output)
	}
	session.broadcastOutput([]byte(strings.Repeat("x", terminalLogLimit+100)))
	session.broadcastOutput([]byte("recent output"))
	output := receiveTerminalReplay(t, server)
	if len(output) != terminalLogLimit || !strings.HasSuffix(output, "recent output") {
		t.Fatal("reconnect replay must retain only the bounded recent output")
	}
}

func TestDesktopTerminalKeepsStartupOnlyDelivery(t *testing.T) {
	session, server := terminalReplayFixture(t, false)
	session.broadcastOutput([]byte("startup\n"))
	if output := receiveTerminalReplay(t, server); output != "startup\n" {
		t.Fatalf("desktop first connection = %q", output)
	}
	session.broadcastOutput([]byte("later output\n"))
	if output := receiveTerminalReplay(t, server); output != "" {
		t.Fatal("desktop reconnect unexpectedly replayed output")
	}
}

func TestTerminalReplayDoesNotDuplicateConcurrentOutput(t *testing.T) {
	session := &TerminalSession{log: &terminalLog{}, replayOutput: true, clients: make(map[*terminalClient]bool)}
	server := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		client := &terminalClient{conn: conn}
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 100 {
				session.broadcastOutput([]byte("chunk\n"))
			}
			session.broadcastOutput([]byte("finished\n"))
		}()
		session.addClient(client)
		<-done
		session.removeClient(client)
	}))
	defer server.Close()
	conn, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	var output strings.Builder
	for !strings.HasSuffix(output.String(), "finished\n") {
		var frame []byte
		if err := websocket.Message.Receive(conn, &frame); err != nil {
			t.Fatal(err)
		}
		if len(frame) > 0 && frame[0] == terminalBinaryDataFrame {
			output.Write(frame[1:])
		}
	}
	if output.String() != strings.Repeat("chunk\n", 100)+"finished\n" {
		t.Fatal("connecting during output duplicated or dropped chunks")
	}
}

func TestTerminalLogsSurviveExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX test command")
	}
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	session, err := tm.StartWithEnvironment("app", "printf stdout; printf stderr >&2; exit 0", t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	output, truncated := tm.Logs("app")
	if tm.IsRunning("app") || !strings.Contains(output, "stdout") || !strings.Contains(output, "stderr") || truncated {
		t.Fatalf("exited process lost output: %q %v", output, truncated)
	}
	if other, _ := tm.Logs("other"); other != "" {
		t.Fatal("logs leaked to another terminal")
	}
}

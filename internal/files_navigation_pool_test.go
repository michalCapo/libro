package libro

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNavigationServerReuseRestartAndDiskChanges(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not installed")
	}
	t.Cleanup(closeNavigationServers)
	root := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("tsconfig.json", `{"compilerOptions":{"noEmit":true}}`)
	source := "export class App {\n link() { return this; }\n}\n"
	write("app.ts", source)
	write("dev.ts", "import {App} from './app';\nnew App().link();\n")
	navigate := func(file, kind string, line, column int) fileResult {
		t.Helper()
		result, err := navigateProjectFile(root, file, kind, "", 0, line, column)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	key := root + "\x00typescript-language-server\x00--stdio"
	getSession := func() *navigationSession {
		t.Helper()
		navigationPool.Lock()
		defer navigationPool.Unlock()
		return navigationPool.sessions[key]
	}
	first := navigate("dev.ts", "definition", 2, 11)
	if len(first.Matches) != 1 || first.Matches[0].Line != 2 {
		t.Fatalf("definition: %+v", first.Matches)
	}
	session := getSession()
	server := session.server
	navigate("dev.ts", "references", 2, 11)
	if session.server != server {
		t.Fatal("server was not reused")
	}
	// Open the target, then change it on disk. Refresh its existing LSP overlay.
	navigate("app.ts", "definition", 2, 2)
	write("app.ts", "// external edit\n"+source)
	result := navigate("dev.ts", "definition", 2, 11)
	if len(result.Matches) != 1 || result.Matches[0].Line != 3 {
		t.Fatalf("stale definition after disk edit: %+v", result.Matches)
	}
	navigate("dev.ts", "restart", 2, 11)
	if session.server == server || server.cmd.ProcessState == nil {
		t.Fatal("restart did not replace and reap old server")
	}
	server = session.server
	server.cancel()
	select {
	case <-server.connection.DisconnectNotify():
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit")
	}
	navigate("dev.ts", "definition", 2, 11)
	if session.server == server {
		t.Fatal("crashed server was not replaced")
	}
	// A stale idle timer must not shut down a recently used server.
	retireNavigationSession(key, session, true)
	if getSession() != session {
		t.Fatal("active server was retired")
	}
	navigationPool.Lock()
	session.lastUsed = time.Now().Add(-navigationIdleTimeout)
	navigationPool.Unlock()
	server = session.server
	retireNavigationSession(key, session, true)
	if getSession() != nil || server.cmd.ProcessState == nil {
		t.Fatal("idle server was not removed and reaped")
	}
}

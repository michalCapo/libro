package libro

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"libro/internal/components"
)

func TestWorkspaceScripts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	for name, script := range map[string]string{
		"keyboard": keyboardShortcutsJS("test-session"),
		"browser":  components.BrowserJS(),
		"address":  urlPopupJS("test-session"),
		"palette":  commandPopupJS("test-session"),
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(node, "--check")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("invalid JavaScript: %v\n%s", err, out)
			}
		})
	}
}

func TestInitialWorkspaceNavigation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	for _, tc := range []struct{ name, hash, active, want string }{
		{"URL project", "#project", "other", "project"},
		{"restored workspace", "", "thread:saved", "thread:saved"},
		{"empty workspace", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := fmt.Sprintf(`
const assert = require('node:assert/strict');
const calls = [];
const location = {hash:%q};
const window = {__libroActiveProject:%q};
const history = {replaceState(){}};
const document = {};
function addEventListener(){}
function setTimeout(fn){fn();}
const __ws = {connected:()=>true, call:(name,input)=>calls.push([name,input])};
`, tc.hash, tc.active) + initHashJS("test-session") + fmt.Sprintf(`
const expected = %q;
assert.deepEqual(calls, expected ? [['project.switch', {sid:'test-session',name:expected}]] : []);
`, tc.want)
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("initial navigation failed: %v\n%s", err, out)
			}
		})
	}
}

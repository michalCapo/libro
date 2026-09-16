package libro

import (
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
			for _, removed := range []string{"project.select", "nav.slot", "zen.toggle", "project.apps.save", "project.apps.open", "project.apps.clean", "history.delete", "app.run.execute", "__libroOpenSearch"} {
				if strings.Contains(script, removed) {
					t.Errorf("removed feature remains: %s", removed)
				}
			}
			cmd := exec.Command(node, "--check")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("invalid JavaScript: %v\n%s", err, out)
			}
		})
	}
}

package libro

import "testing"

func TestToolShortcutValidation(t *testing.T) {
	keys := defaultToolKeybindings()
	if err := validateToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
	keys["nvim"] = keys["terminal"]
	if validateToolKeybindings(keys) == nil {
		t.Fatal("accepted duplicate")
	}
	keys["nvim"] = ""
	if err := validateToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"A", "Shift+A", "Ctrl+A", "Ctrl+Alt+", "Ctrl+`", "Ctrl+Ctrl+A"} {
		keys["nvim"] = invalid
		if validateToolKeybindings(keys) == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
}

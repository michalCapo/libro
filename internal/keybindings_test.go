package libro

import "testing"

func TestToolShortcutValidation(t *testing.T) {
	keys := defaultToolKeybindings()
	if keys["voice"] != "Tab" {
		t.Fatalf("voice shortcut = %q", keys["voice"])
	}
	if keys["settings"] != "Ctrl+Shift+S" {
		t.Fatalf("settings shortcut = %q", keys["settings"])
	}
	if keys["panel-size-down"] != "Ctrl+." {
		t.Fatalf("panel-size-down shortcut = %q", keys["panel-size-down"])
	}
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

func TestCustomToolShortcuts(t *testing.T) {
	keys := defaultToolKeybindings()
	keys["custom-tool-docs"] = "Alt+D"
	if err := validateToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Ctrl+T", "Ctrl+1", "Ctrl+Shift+9", "Shift+D"} {
		keys["custom-tool-docs"] = key
		if validateToolKeybindings(keys) == nil {
			t.Fatalf("accepted %s", key)
		}
	}
	keys["custom-tool-docs"] = ""
	if err := validateToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
}

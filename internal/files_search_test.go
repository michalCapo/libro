package libro

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProjectSearch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{".gitignore": "ignored.txt\n", "main.go": "package main\n// 😀 needle\nfunc main() {}\n", "ignored.txt": "needle", ".hidden": "needle", "binary": "\x00needle"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(external, []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	result, err := searchProjectFiles(root, "needle", false, false)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("search: %+v, %v", result, err)
	}
	match := result.Matches[0]
	if match.Path != "main.go" || match.Line != 2 || match.Column != 6 {
		t.Fatalf("UTF-16 match position: %+v", match)
	}
	result, err = searchProjectFiles(root, "needle", true, false)
	if err != nil || len(result.Matches) != 3 {
		t.Fatalf("include ignored: %+v, %v", result, err)
	}
	result, err = searchProjectFiles(root, "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Entries {
		if entry.Path == "ignored.txt" || entry.Path == ".hidden" || entry.Path == "linked.txt" {
			t.Fatalf("unexpected indexed file: %s", entry.Path)
		}
	}
	for _, query := range []string{"not found", "--help", "$(echo needle)", "[needle]"} {
		result, err = searchProjectFiles(root, query, false, false)
		if err != nil || len(result.Matches) != 0 {
			t.Fatalf("literal query %q: %+v, %v", query, result, err)
		}
	}
}

func TestFilePreviewRefresh(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := readProjectFile(root, "source.go")
	if err != nil || original.Version == "" {
		t.Fatalf("initial read: %+v %v", original, err)
	}
	unchanged, err := readProjectFileVersion(root, "source.go", original.Version)
	if err != nil || !unchanged.Unchanged || unchanged.Text != "" {
		t.Fatalf("unchanged read: %+v %v", unchanged, err)
	}
	if err := os.WriteFile(path, []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := readProjectFileVersion(root, "source.go", original.Version)
	if err != nil || changed.Unchanged || changed.Text != "changed source" {
		t.Fatalf("changed read: %+v %v", changed, err)
	}
}

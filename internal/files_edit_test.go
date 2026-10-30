package libro

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveProjectFile(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "run.sh")
	if err := os.WriteFile(name, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := readProjectFile(root, "run.sh")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := saveProjectFile(root, "run.sh", "new text", file.Version)
	if err != nil || saved.Text != "new text" || saved.Version == file.Version {
		t.Fatalf("save: %+v %v", saved, err)
	}
	if info, err := os.Stat(name); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode: %v %v", info, err)
	}
	if _, err := saveProjectFile(root, "run.sh", "stale", file.Version); err == nil {
		t.Fatal("overwrote a file that changed on disk")
	}
	if _, err := saveProjectFile(root, "run.sh", "forced", ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(name); string(data) != "forced" {
		t.Fatalf("content = %q", data)
	}
	if err := os.Symlink("run.sh", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := saveProjectFile(root, "link", "through link", ""); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(root, "link")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("save replaced the link")
	}
	if data, _ := os.ReadFile(name); string(data) != "through link" {
		t.Fatalf("content = %q", data)
	}
	if _, err := saveProjectFile(root, "link", "atomic", ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(name); string(data) != "atomic" {
		t.Fatalf("content = %q", data)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 2 {
		t.Fatalf("left temporary files: %v", entries)
	}
	for _, path := range []string{"", ".", "../outside", "/etc/passwd", "sub/../run.sh"} {
		if _, err := saveProjectFile(root, path, "x", ""); err == nil {
			t.Fatalf("saved %q", path)
		}
	}
}

func TestProjectEntries(t *testing.T) {
	root := t.TempDir()
	if err := createProjectEntry(root, "src/app/main.go", false); err != nil {
		t.Fatal(err)
	}
	if err := createProjectEntry(root, "src/app/main.go", false); err == nil {
		t.Fatal("created an existing file")
	}
	if err := createProjectEntry(root, "docs", true); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "docs")); err != nil || !info.IsDir() {
		t.Fatalf("folder: %v", err)
	}
	if err := renameProjectEntry(root, "src/app/main.go", "docs"); err == nil {
		t.Fatal("renamed over an existing entry")
	}
	if err := renameProjectEntry(root, "src/app/main.go", "cmd/tool/main.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd/tool/main.go")); err != nil {
		t.Fatal(err)
	}
	if err := deleteProjectEntry(root, "cmd"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd")); !os.IsNotExist(err) {
		t.Fatalf("folder still exists: %v", err)
	}
	if err := deleteProjectEntry(root, "missing"); err == nil {
		t.Fatal("deleted a missing entry")
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "keep.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", "../outside", "docs/..", "docs/../docs"} {
		if createProjectEntry(root, path, true) == nil || renameProjectEntry(root, "docs", path) == nil || deleteProjectEntry(root, path) == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}

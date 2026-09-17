package libro

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectFiles(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "src"), 0700)
	os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0600)
	os.WriteFile(filepath.Join(root, "binary"), []byte{0, 1, 2}, 0600)
	listing, err := readProjectFile(root, "")
	if err != nil || !listing.Directory || len(listing.Entries) != 3 || listing.Entries[0].Name != "src" {
		t.Fatalf("listing: %+v %v", listing, err)
	}
	file, err := readProjectFile(root, "hello.txt")
	if err != nil || file.Text != "hello" {
		t.Fatalf("preview: %+v %v", file, err)
	}
	for _, path := range []string{"../outside", "/etc/passwd", "binary"} {
		if _, err := readProjectFile(root, path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	os.WriteFile(outside, []byte("outside"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := readProjectFile(root, "escape"); err == nil {
		t.Fatal("followed escaping symlink")
	}
}

func TestProjectFileToOpen(t *testing.T) {
	root := t.TempDir()
	name := ".hidden file.bin"
	if err := os.WriteFile(filepath.Join(root, name), []byte{0, 1, 2}, 0600); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	got, err := projectFileToOpen(root, name)
	if err != nil || got != want {
		t.Fatalf("open path = %q, %v; want %q", got, err, want)
	}
	for _, path := range []string{"", ".", "../outside", want, "missing"} {
		if _, err := projectFileToOpen(root, path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := projectFileToOpen(root, "escape"); err == nil {
		t.Fatal("accepted escaping symlink")
	}
	if err := os.Symlink(name, filepath.Join(root, "local-link")); err != nil {
		t.Fatal(err)
	}
	if got, err := projectFileToOpen(root, "local-link"); err != nil || got != want {
		t.Fatalf("local symlink = %q, %v; want %q", got, err, want)
	}
}

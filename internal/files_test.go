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

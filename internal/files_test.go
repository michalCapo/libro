package libro

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary"), []byte{0, 1, 2}, 0600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
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

func TestProjectMediaPreview(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct{ name, data, mime string }{
		{"photo.png", "\x89PNG\r\n\x1a\n", "image/png"},
		{"vector.svg", "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "image/svg+xml"},
		{"document.pdf", "%PDF-1.7", "application/pdf"},
		{"sound.wav", "RIFF", "audio/"},
		{"movie.mp4", "\x00\x00\x00\x18ftypmp42", "video/mp4"},
		{"unknown", "\x89PNG\r\n\x1a\n", "image/png"},
		{"large.pdf", "%PDF-1.7" + strings.Repeat("x", 1024*1024), "application/pdf"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, test.name), []byte(test.data), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := readProjectFile(root, test.name)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.StdEncoding.DecodeString(result.Data)
			if err != nil || string(decoded) != test.data || !strings.HasPrefix(result.MIME, test.mime) || result.Text != "" {
				t.Fatalf("incorrect media preview: MIME=%q, decode error=%v", result.MIME, err)
			}
		})
	}
	for _, test := range []struct {
		name string
		size int
	}{
		{"large.txt", 1024*1024 + 1}, {"oversized.pdf", 32*1024*1024 + 1},
	} {
		f, err := os.Create(filepath.Join(root, test.name))
		if err != nil {
			t.Fatal(err)
		}
		err = f.Truncate(int64(test.size))
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readProjectFile(root, test.name); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("%s size limit: %v", test.name, err)
		}
	}
}

func TestFilesParentRoot(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "project")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "sibling.txt"), []byte("outside project"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := filesRoot(project, 0); got != project {
		t.Fatalf("initial root = %s", got)
	}
	root := filesRoot(project, 1)
	if root != parent {
		t.Fatalf("parent root = %s, want %s", root, parent)
	}
	result, err := readProjectFile(root, "sibling.txt")
	if err != nil || result.Text != "outside project" {
		t.Fatalf("preview outside project: %v, %q", err, result.Text)
	}
	if _, err := projectFileToOpen(root, "sibling.txt"); err != nil {
		t.Fatalf("open outside project: %v", err)
	}
	if _, err := readProjectFile(root, "../escape.txt"); err == nil {
		t.Fatal("relative file requests must stay inside the current root")
	}
	top := filesRoot(project, 1024)
	if filesRoot(top, 1) != top {
		t.Fatal("parent of filesystem root must stay at root")
	}
}

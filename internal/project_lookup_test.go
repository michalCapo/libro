package libro

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectLookupExactDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"nisa/assets", "nisa-new", "nisa-old"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	matches := projectDirLookup(context.Background(), filepath.Join(root, "nisa"))
	if len(matches) != 3 || matches[0].Name != "nisa" {
		t.Fatalf("exact folder should stay first: %+v", matches)
	}
	matches = projectDirLookup(context.Background(), filepath.Join(root, "nisa")+string(os.PathSeparator))
	if len(matches) != 2 || matches[0].Name != ".." || matches[1].Name != "assets" {
		t.Fatalf("slash should browse children: %+v", matches)
	}
}

func TestProjectLookupCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if matches := projectDirLookup(ctx, "nisa"); len(matches) != 0 {
		t.Fatalf("canceled lookup returned %+v", matches)
	}
}

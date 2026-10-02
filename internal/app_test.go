package libro

import "testing"

func TestEnsureSchemePreservesFileURL(t *testing.T) {
	for _, localFile := range []string{"file:///home/capo/plan.html", "file:///home/capo/local file.html", "File:///home/capo/plan.html"} {
		if got := ensureScheme(localFile); got != localFile {
			t.Errorf("ensureScheme(%q) = %q", localFile, got)
		}
	}
}

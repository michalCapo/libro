package libro

import "testing"

func TestEnsureSchemePreservesFileURL(t *testing.T) {
	const localFile = "file:///home/capo/Videos/tanecky/plan.html"

	if got := ensureScheme(localFile); got != localFile {
		t.Fatalf("ensureScheme(%q) = %q, want %q", localFile, got, localFile)
	}
}

func TestEnsureSchemePreservesFileURLWithSpaces(t *testing.T) {
	const localFile = "file:///home/capo/Videos/local file.html"

	if got := ensureScheme(localFile); got != localFile {
		t.Fatalf("ensureScheme(%q) = %q, want %q", localFile, got, localFile)
	}
}

func TestEnsureSchemePreservesFileURLCaseInsensitive(t *testing.T) {
	const localFile = "File:///home/capo/Videos/tanecky/plan.html"

	if got := ensureScheme(localFile); got != localFile {
		t.Fatalf("ensureScheme(%q) = %q, want %q", localFile, got, localFile)
	}
}

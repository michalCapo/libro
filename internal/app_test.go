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

func TestValidLibroSessionID(t *testing.T) {
	valid := []string{"session-1", "session-123456"}
	for _, sid := range valid {
		if !validLibroSessionID(sid) {
			t.Fatalf("validLibroSessionID(%q) = false, want true", sid)
		}
	}

	invalid := []string{"", "session-", "session-abc", "other-1", "session-1/path"}
	for _, sid := range invalid {
		if validLibroSessionID(sid) {
			t.Fatalf("validLibroSessionID(%q) = true, want false", sid)
		}
	}
}

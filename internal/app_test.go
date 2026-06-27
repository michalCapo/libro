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

func TestFaviconURLUsesWorkingGoogleEndpoint(t *testing.T) {
	got := faviconURL("https://discord.com/channels", 32)
	want := "https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=https%3A%2F%2Fdiscord.com&size=32"
	if got != want {
		t.Fatalf("faviconURL() = %q, want %q", got, want)
	}
}

func TestFaviconURLAddsSchemeForBareDomains(t *testing.T) {
	got := faviconURL("discord.com", 16)
	want := "https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=https%3A%2F%2Fdiscord.com&size=16"
	if got != want {
		t.Fatalf("faviconURL() = %q, want %q", got, want)
	}
}

func TestFaviconURLSkipsFileURLs(t *testing.T) {
	if got := faviconURL("file:///home/capo/site.html", 32); got != "" {
		t.Fatalf("faviconURL(file URL) = %q, want empty", got)
	}
}

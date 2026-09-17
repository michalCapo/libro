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

func TestFaviconURLSkipsLocalAddresses(t *testing.T) {
	for _, address := range []string{"http://localhost:1411", "http://app.localhost:3000", "http://127.0.0.1:8100", "http://[::1]:8100", "http://192.168.1.2", "http://intranet"} {
		if got := faviconURL(address, 32); got != "" {
			t.Errorf("faviconURL(%q) = %q, want empty", address, got)
		}
	}
}

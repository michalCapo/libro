package libro

import (
	"embed"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceIsolation(t *testing.T) {
	t.Setenv("LIBRO_INSTANCE", "")
	t.Setenv("LIBRO_PORT", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := ConfigureInstance("", ""); err != nil {
		t.Fatal(err)
	}
	regular := dbFilePath()
	if Port() != "8100" {
		t.Fatal(Port())
	}
	if err := os.WriteFile(regular, []byte("regular data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureInstance("dev", ""); err != nil {
		t.Fatal(err)
	}
	dev := dbFilePath()
	if Port() != "8101" || dev != filepath.Join(filepath.Dir(regular), "instances", "dev", "libro.db") {
		t.Fatalf("%s %s", Port(), dev)
	}
	if _, err := os.Stat(dev); !os.IsNotExist(err) {
		t.Fatalf("dev must start empty: %v", err)
	}
	if err := ConfigureInstance("feature-a", "8102"); err != nil {
		t.Fatal(err)
	}
	if Port() != "8102" || dbFilePath() == dev {
		t.Fatal("instance not isolated")
	}
	data, err := os.ReadFile(regular)
	if err != nil || string(data) != "regular data" {
		t.Fatal("regular data changed", err)
	}
}

func TestInvalidInstanceConfiguration(t *testing.T) {
	for _, name := range []string{"../dev", "a/b", "a\\b", ".", "DEV", strings.Repeat("a", 65)} {
		if err := ConfigureInstance(name, "8101"); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	for _, port := range []string{"0", "-1", "65536", "abc"} {
		if err := ConfigureInstance("dev", port); err == nil {
			t.Errorf("accepted port %q", port)
		}
	}
}

func TestOccupiedPortFailsBeforeDatabase(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIBRO_PORT", port)
	t.Setenv("LIBRO_INSTANCE", "dev")
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	if err := Run(embed.FS{}, false); err == nil {
		t.Fatal("expected occupied port error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("startup touched data before binding", err)
	}
}

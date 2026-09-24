package libro

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestProjectCommandPersistence(t *testing.T) {
	original := db
	path := filepath.Join(t.TempDir(), "settings.db")
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	if err := setProjectCommand("/project/a", "  bun src/dev.ts  "); err != nil {
		t.Fatal(err)
	}
	if err := setProjectCommand("/project/b", "air"); err != nil {
		t.Fatal(err)
	}
	if err := setProjectCommand("/project/a", "bad\x00command"); err == nil {
		t.Fatal("accepted null character")
	}
	if err := saveApplicationSettings("/project/a", applicationSettings{Mode: "thread", Port: 4321}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []applicationSettings{{Mode: "other"}, {Mode: "thread", Port: -1}, {Mode: "thread", Port: 65536}} {
		if err := saveApplicationSettings("/project/a", invalid); err == nil {
			t.Fatal("invalid application settings accepted")
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loadApplicationSettings("/project/a"); got.Mode != "thread" || got.Port != 4321 {
		t.Fatalf("application settings did not persist: %+v", got)
	}
	if got := loadApplicationSettings("/project/b"); got.Mode != "thread" || got.Port != 0 {
		t.Fatal("unconfigured projects should default to per-thread mode")
	}
	if projectCommand("/project/a") != "bun src/dev.ts" || projectCommand("/project/b") != "air" || projectCommand("/project/unknown") != "" {
		t.Fatal("commands were not saved per folder")
	}
	if err := setProjectCommand("/project/a", ""); err != nil {
		t.Fatal(err)
	}
	if projectCommand("/project/a") != "" || projectCommand("/project/b") != "air" {
		t.Fatal("clearing command affected another project")
	}
}

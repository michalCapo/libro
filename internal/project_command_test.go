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
	t.Cleanup(func() { db.Close(); db = original })
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
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
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

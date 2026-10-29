package libro

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestForeignKeysOnEveryConnection(t *testing.T) {
	pool, err := sql.Open("sqlite", dbDSN(filepath.Join(t.TempDir(), "libro.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()
	if _, err := pool.Exec("CREATE TABLE parent (id INTEGER PRIMARY KEY); CREATE TABLE child (parent_id INTEGER REFERENCES parent(id))"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Hold the first connection so the insert runs on a second one.
	first, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	for name, conn := range map[string]*sql.Conn{"first": first, "second": second} {
		var on int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil || on != 1 {
			t.Errorf("%s connection: foreign_keys = %d, %v", name, on, err)
		}
	}
	if _, err := second.ExecContext(ctx, "INSERT INTO child (parent_id) VALUES (42)"); err == nil {
		t.Error("second connection accepted a child without its parent")
	}
}

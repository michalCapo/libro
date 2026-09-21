package libro

import (
	"database/sql"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	dbMu sync.Mutex
)

// InitDB opens (or creates) the instance database and creates tables.
func InitDB() {
	dbPath := dbFilePath()
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("db: failed to open %s: %v", dbPath, err)
	}
	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Printf("db: WAL pragma failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		log.Printf("db: foreign_keys pragma failed: %v", err)
	}

	createTables()
	if err := os.Chmod(dbPath, 0o600); err != nil {
		log.Printf("db: failed to restrict permissions on %s: %v", dbPath, err)
	}
	removeDefaultHomeProject()
}

func dbFilePath() string {
	dir, err := libroDataDir()
	if err != nil {
		log.Printf("db: failed to resolve data dir: %v", err)
		return "libro.db"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("db: failed to create data dir %s: %v", dir, err)
		return filepath.Join(dir, "libro.db")
	}
	dbPath := filepath.Join(dir, "libro.db")
	if os.Getenv("LIBRO_INSTANCE") == "" {
		migrateLegacyDB(dbPath)
	}
	return dbPath
}

func libroDataDir() (string, error) {
	dir, err := libroBaseDataDir()
	if err != nil {
		return "", err
	}
	return instanceDir(dir), nil
}

func libroBaseDataDir() (string, error) {
	if dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dataHome != "" {
		return filepath.Join(dataHome, "libro"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "libro"), nil
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			return filepath.Join(appData, "libro"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "libro"), nil
	default:
		return filepath.Join(home, ".local", "share", "libro"), nil
	}
}

func migrateLegacyDB(dbPath string) {
	if fileExists(dbPath) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	legacyPath := filepath.Join(filepath.Dir(exe), "libro.db")
	if legacyPath == dbPath || !fileExists(legacyPath) {
		return
	}

	if err := copyFile(legacyPath, dbPath); err != nil {
		log.Printf("db: failed to migrate legacy db from %s to %s: %v", legacyPath, dbPath, err)
		return
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		src := legacyPath + suffix
		dst := dbPath + suffix
		if !fileExists(src) {
			continue
		}
		if err := copyFile(src, dst); err != nil {
			log.Printf("db: failed to migrate legacy db sidecar %s: %v", src, err)
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func createTables() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS notes (
            id TEXT PRIMARY KEY,
            project TEXT NOT NULL,
            title TEXT NOT NULL,
            body TEXT NOT NULL,
            state TEXT NOT NULL CHECK(state IN ('new', 'archived')),
            updated TEXT NOT NULL,
            images TEXT NOT NULL DEFAULT '[]'
        );
        CREATE TABLE IF NOT EXISTS threads (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            archived INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS projects (
			name     TEXT PRIMARY KEY,
			path     TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);

	`)
	if err != nil {
		log.Fatalf("db: failed to create tables: %v", err)
	}
	// Add resume metadata to existing installations as well as new databases.
	for _, column := range []string{"session_id", "agent_id", "agent_command"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('threads') WHERE name = ?", column).Scan(&count); err != nil {
			log.Fatalf("db: inspect threads: %v", err)
		}
		if count == 0 {
			if _, err := db.Exec("ALTER TABLE threads ADD COLUMN " + column + " TEXT NOT NULL DEFAULT ''"); err != nil {
				log.Fatalf("db: migrate threads: %v", err)
			}
		}
	}
}

// Remove the old automatically seeded project once, preserving later user additions.
func removeDefaultHomeProject() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		log.Printf("db: migrate default project: %v", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`DELETE FROM projects WHERE name = 'home' AND path = ? AND position = 0
		AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'default_home_removed')`, home); err == nil {
		_, err = tx.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('default_home_removed', 'true')`)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Printf("db: migrate default project: %v", err)
	}
}

// CloseDB closes the database connection.
func CloseDB() {
	if db != nil {
		_ = db.Close()
	}
}

// --- Project CRUD ---

// DBLoadProjects returns all projects ordered by position.
func DBLoadProjects() []Project {
	dbMu.Lock()
	defer dbMu.Unlock()

	rows, err := db.Query("SELECT name, path FROM projects ORDER BY position, rowid")
	if err != nil {
		log.Printf("db: load projects: %v", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Name, &p.Path); err != nil {
			continue
		}
		projects = append(projects, p)
	}
	return projects
}

// DBSaveProject inserts or updates a project, preserving its slot when replacing.
func DBSaveProject(name, path string) {
	dbMu.Lock()
	defer dbMu.Unlock()

	var existingPosition int
	err := db.QueryRow("SELECT position FROM projects WHERE name = ?", name).Scan(&existingPosition)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("db: lookup project %s: %v", name, err)
		return
	}

	position := existingPosition
	if err == sql.ErrNoRows {
		_ = db.QueryRow("SELECT COALESCE(MAX(position),0) FROM projects").Scan(&position)
		position++
	}

	_, err = db.Exec(
		"INSERT OR REPLACE INTO projects (name, path, position) VALUES (?, ?, ?)",
		name, path, position,
	)
	if err != nil {
		log.Printf("db: save project %s: %v", name, err)
	}
}

// DBFindProjectByPath returns the most specific project whose path
// matches the given working directory exactly or as an ancestor.
func DBFindProjectByPath(path string) (Project, bool) {
	projects := DBLoadProjects()
	cleanPath := filepath.Clean(path)

	var best Project
	bestLen := -1
	for _, p := range projects {
		projectPath := filepath.Clean(p.Path)
		if cleanPath != projectPath && !strings.HasPrefix(cleanPath, projectPath+string(os.PathSeparator)) {
			continue
		}
		if len(projectPath) > bestLen {
			best = p
			bestLen = len(projectPath)
		}
	}

	return best, bestLen >= 0
}

// DBRemoveProject deletes a project and its apps (via CASCADE).
func DBRemoveProject(name string) {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM projects WHERE name = ?", name)
	if err != nil {
		log.Printf("db: remove project %s: %v", name, err)
	}
}

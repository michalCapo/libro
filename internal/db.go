package libro

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// SavedApp represents a persisted application definition (no runtime state like Port/ID).
type SavedApp struct {
	Type     string // "url" or "terminal"
	URL      string
	Command  string
	Width    string
	Writable bool
	Name     string
	IconURL  string // cached icon URL discovered via web search
}

var (
	db   *sql.DB
	dbMu sync.Mutex
)

// InitDB opens (or creates) libro.db next to the running binary and creates tables.
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
	migrateTables()
	ensureHomeProject()
}

func dbFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "libro.db"
	}
	return filepath.Join(filepath.Dir(exe), "libro.db")
}

func createTables() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			name     TEXT PRIMARY KEY,
			path     TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS browsed_urls (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			url        TEXT NOT NULL UNIQUE,
			title      TEXT NOT NULL DEFAULT '',
			visited_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS saved_apps (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			project_name TEXT NOT NULL DEFAULT 'home',
			type         TEXT NOT NULL,
			url          TEXT NOT NULL DEFAULT '',
			command      TEXT NOT NULL DEFAULT '',
			width        TEXT NOT NULL DEFAULT 'lg',
			writable     INTEGER NOT NULL DEFAULT 1,
			name         TEXT NOT NULL DEFAULT '',
			icon_url     TEXT NOT NULL DEFAULT '',
			position     INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (project_name) REFERENCES projects(name) ON DELETE CASCADE
		);
	`)
	if err != nil {
		log.Fatalf("db: failed to create tables: %v", err)
	}
}

// migrateTables adds columns that may be missing in older databases.
func migrateTables() {
	// Add icon_url column if it doesn't exist (added for icon discovery feature)
	_, _ = db.Exec("ALTER TABLE saved_apps ADD COLUMN icon_url TEXT NOT NULL DEFAULT ''")
}

func ensureHomeProject() {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO projects (name, path, position) VALUES ('home', ?, 0)`, home)
	if err != nil {
		log.Printf("db: failed to ensure home project: %v", err)
	}
}

// CloseDB closes the database connection.
func CloseDB() {
	if db != nil {
		db.Close()
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
		return []Project{{Name: "home", Path: defaultHomeDir()}}
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Name, &p.Path); err != nil {
			continue
		}
		projects = append(projects, p)
	}
	if len(projects) == 0 {
		return []Project{{Name: "home", Path: defaultHomeDir()}}
	}
	return projects
}

// DBSaveProject inserts or updates a project.
func DBSaveProject(name, path string) {
	dbMu.Lock()
	defer dbMu.Unlock()

	// Get max position
	var maxPos int
	_ = db.QueryRow("SELECT COALESCE(MAX(position),0) FROM projects").Scan(&maxPos)

	_, err := db.Exec(
		"INSERT OR REPLACE INTO projects (name, path, position) VALUES (?, ?, ?)",
		name, path, maxPos+1,
	)
	if err != nil {
		log.Printf("db: save project %s: %v", name, err)
	}
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

// --- Saved Apps CRUD ---

// DBLoadSavedApps returns saved apps for a project, ordered by position.
func DBLoadSavedApps(projectName string) []SavedApp {
	dbMu.Lock()
	defer dbMu.Unlock()

	rows, err := db.Query(
		"SELECT type, url, command, width, writable, name, icon_url FROM saved_apps WHERE project_name = ? ORDER BY position, id",
		projectName,
	)
	if err != nil {
		log.Printf("db: load saved apps for %s: %v", projectName, err)
		return nil
	}
	defer rows.Close()

	var apps []SavedApp
	for rows.Next() {
		var a SavedApp
		var writable int
		if err := rows.Scan(&a.Type, &a.URL, &a.Command, &a.Width, &writable, &a.Name, &a.IconURL); err != nil {
			continue
		}
		a.Writable = writable != 0
		apps = append(apps, a)
	}
	return apps
}

// DBLoadAllSavedApps returns all saved apps across all projects.
func DBLoadAllSavedApps() []SavedApp {
	dbMu.Lock()
	defer dbMu.Unlock()

	rows, err := db.Query("SELECT type, url, command, width, writable, name, icon_url FROM saved_apps ORDER BY position, id")
	if err != nil {
		log.Printf("db: load all saved apps: %v", err)
		return nil
	}
	defer rows.Close()

	var apps []SavedApp
	for rows.Next() {
		var a SavedApp
		var writable int
		if err := rows.Scan(&a.Type, &a.URL, &a.Command, &a.Width, &writable, &a.Name, &a.IconURL); err != nil {
			continue
		}
		a.Writable = writable != 0
		apps = append(apps, a)
	}
	return apps
}

// DBAddSavedApp appends a saved app to a project.
func DBAddSavedApp(projectName string, app SavedApp) {
	dbMu.Lock()
	defer dbMu.Unlock()

	var maxPos int
	_ = db.QueryRow("SELECT COALESCE(MAX(position),0) FROM saved_apps WHERE project_name = ?", projectName).Scan(&maxPos)

	writable := 0
	if app.Writable {
		writable = 1
	}
	_, err := db.Exec(
		"INSERT INTO saved_apps (project_name, type, url, command, width, writable, name, icon_url, position) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		projectName, app.Type, app.URL, app.Command, app.Width, writable, app.Name, app.IconURL, maxPos+1,
	)
	if err != nil {
		log.Printf("db: add saved app: %v", err)
	}
}

// DBUpdateSavedApp updates a saved app at a given index (0-based) within a project.
func DBUpdateSavedApp(projectName string, index int, app SavedApp) {
	dbMu.Lock()
	defer dbMu.Unlock()

	// Find the app ID at this position index
	id := dbAppIDAtIndex(projectName, index)
	if id < 0 {
		return
	}

	writable := 0
	if app.Writable {
		writable = 1
	}
	_, err := db.Exec(
		"UPDATE saved_apps SET type=?, url=?, command=?, width=?, writable=?, name=?, icon_url=? WHERE id=?",
		app.Type, app.URL, app.Command, app.Width, writable, app.Name, app.IconURL, id,
	)
	if err != nil {
		log.Printf("db: update saved app at %d: %v", index, err)
	}
}

// DBUpdateSavedAppURL updates the URL of a saved app at a given index.
func DBUpdateSavedAppURL(projectName string, index int, newURL string) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := dbAppIDAtIndex(projectName, index)
	if id < 0 {
		return
	}

	_, err := db.Exec("UPDATE saved_apps SET url=? WHERE id=?", newURL, id)
	if err != nil {
		log.Printf("db: update saved app url at %d: %v", index, err)
	}
}

// DBRemoveSavedApp removes a saved app at a given index within a project.
func DBRemoveSavedApp(projectName string, index int) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := dbAppIDAtIndex(projectName, index)
	if id < 0 {
		return
	}

	_, err := db.Exec("DELETE FROM saved_apps WHERE id=?", id)
	if err != nil {
		log.Printf("db: remove saved app at %d: %v", index, err)
	}
}

// DBRemoveSavedAppsByProject removes all saved apps for a project.
func DBRemoveSavedAppsByProject(projectName string) {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM saved_apps WHERE project_name=?", projectName)
	if err != nil {
		log.Printf("db: remove saved apps for %s: %v", projectName, err)
	}
}

// dbAppIDAtIndex returns the database row ID for a saved app at a 0-based index. Must be called with dbMu held.
func dbAppIDAtIndex(projectName string, index int) int64 {
	rows, err := db.Query("SELECT id FROM saved_apps WHERE project_name=? ORDER BY position, id", projectName)
	if err != nil {
		return -1
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return -1
		}
		if i == index {
			return id
		}
		i++
	}
	return -1
}

// --- Browsed URLs ---

// DBSaveBrowsedURL upserts a URL into the browsed_urls table (updates visited_at if exists).
func DBSaveBrowsedURL(urlStr string) {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec(
		"INSERT INTO browsed_urls (url, visited_at) VALUES (?, CURRENT_TIMESTAMP) ON CONFLICT(url) DO UPDATE SET visited_at=CURRENT_TIMESTAMP",
		urlStr,
	)
	if err != nil {
		log.Printf("db: save browsed url: %v", err)
	}
}

// DBLoadBrowsedURLs returns all browsed URLs ordered by most recently visited.
func DBLoadBrowsedURLs() []string {
	dbMu.Lock()
	defer dbMu.Unlock()

	rows, err := db.Query("SELECT url FROM browsed_urls ORDER BY visited_at DESC LIMIT 200")
	if err != nil {
		log.Printf("db: load browsed urls: %v", err)
		return nil
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			continue
		}
		urls = append(urls, u)
	}
	return urls
}

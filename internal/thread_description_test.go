package libro

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func descriptionFixture(t *testing.T) (sid, path string, state *AppState) {
	t.Helper()
	oldDB, oldSM := db, sm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "libro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db, sm = oldDB, oldSM })
	createTables()
	path = t.TempDir()
	state = &AppState{
		ActiveProject: "repo/branch",
		Projects:      []Project{{Name: "repo/branch", Path: path, Virtual: true}},
		Apps: []Application{
			{ID: "first", Type: AppTypeTerminal, PluginID: "claude", Dock: "center", SessionID: "one"},
			{ID: "second", Type: AppTypeTerminal, PluginID: "codex", Dock: "center", SessionID: "other"},
		},
		snapshots: make(map[string]*projectSnapshot),
	}
	sm = NewStateManager()
	sm.states["test"] = state
	return "test", path, state
}

func describeOK(t *testing.T, sid, panel, description string) {
	t.Helper()
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "thread", Panel: panel, Thread: threadCommand{Action: "describe", Description: description}}); err != nil {
		t.Fatal(err)
	}
}

func TestDescribeOutranksGeneratedTitlesInItsSession(t *testing.T) {
	sid, path, state := descriptionFixture(t)
	title := func(name string) {
		applyWorktreeTitle(state, actionWorktreeTitleInput{AppID: "first", Project: "repo/branch", Name: name})
	}
	title("Shared email link")
	describeOK(t, sid, "first", "  Teams page:\nlabels ")
	if got := worktreeTitle(path); got != "Teams page: labels" {
		t.Fatalf("description = %q", got)
	}
	// Later titles and a reconnect's replay of an empty title change nothing.
	for _, name := range []string{"Shared email link", ""} {
		title(name)
		if got := worktreeTitle(path); got != "Teams page: labels" {
			t.Fatalf("title %q replaced the description: %q", name, got)
		}
	}
	describeOK(t, sid, "first", "Invoice page")
	if got := worktreeTitle(path); got != "Invoice page" {
		t.Fatalf("second description = %q", got)
	}
	// A new conversation clears it, and the old session cannot claim the next title.
	state.Apps[0].SessionID = "two"
	title("")
	if got := worktreeTitle(path); got != "" {
		t.Fatalf("new session kept the description: %q", got)
	}
	title("New session title")
	state.Apps[0].SessionID = "one"
	title("Old session title")
	if got := worktreeTitle(path); got != "Old session title" {
		t.Fatalf("a cleared description still blocked titles: %q", got)
	}
}

// A browser that was away during the session change only gets the first prompt
// when it reconnects.
func TestFallbackReplacesDescriptionOfEarlierSession(t *testing.T) {
	sid, path, state := descriptionFixture(t)
	describeOK(t, sid, "first", "Old work")
	fallback := actionWorktreeTitleInput{AppID: "first", Project: "repo/branch", Name: "Fix login", Fallback: true}
	applyWorktreeTitle(state, fallback)
	if got := worktreeTitle(path); got != "Old work" {
		t.Fatalf("prompt replaced this session's description: %q", got)
	}
	state.Apps[0].SessionID = "two"
	applyWorktreeTitle(state, fallback)
	if got := worktreeTitle(path); got != "Fix login" {
		t.Fatalf("prompt did not replace the old description: %q", got)
	}
	state.ActiveProject = "thread:plain"
	state.Threads = []Thread{{ID: "thread:plain", Name: "New thread", Path: t.TempDir()}}
	if _, err := db.Exec(`INSERT INTO threads (id, name) VALUES ('thread:plain', 'New thread')`); err != nil {
		t.Fatal(err)
	}
	describeOK(t, sid, "first", "Old work")
	state.Apps[0].SessionID = "three"
	applyThreadTitle(state, actionThreadRenameInput{AppID: "first", ID: "thread:plain", Name: "Fix login", Fallback: true})
	if got := state.thread("thread:plain").Name; got != "Fix login" {
		t.Fatalf("prompt did not replace the old thread description: %q", got)
	}
}

func TestDescribeKeepsThreadNameUntilNewSession(t *testing.T) {
	sid, _, state := descriptionFixture(t)
	state.ActiveProject = "thread:plain"
	state.Threads = []Thread{{ID: "thread:plain", Name: "New thread", Path: t.TempDir()}}
	if _, err := db.Exec(`INSERT INTO threads (id, name) VALUES ('thread:plain', 'New thread')`); err != nil {
		t.Fatal(err)
	}
	describeOK(t, sid, "first", "Teams page")
	rename := func(name string) {
		applyThreadTitle(state, actionThreadRenameInput{AppID: "first", ID: "thread:plain", Name: name})
	}
	rename("Generated title")
	rename("")
	if got := state.thread("thread:plain").Name; got != "Teams page" {
		t.Fatalf("title replaced the description: %q", got)
	}
	// A resumed agent has no panel session yet; the thread still knows it.
	state.Apps[0].SessionID, state.Threads[0].SessionID = "", "one"
	rename("Generated title")
	if got := state.thread("thread:plain").Name; got != "Teams page" {
		t.Fatalf("resumed title replaced the description: %q", got)
	}
	var saved string
	if err := db.QueryRow(`SELECT name FROM threads WHERE id = 'thread:plain'`).Scan(&saved); err != nil || saved != "Teams page" {
		t.Fatalf("saved name = %q: %v", saved, err)
	}
	state.Apps[0].SessionID = "two"
	rename("")
	if got := state.thread("thread:plain").Name; got != "New thread" {
		t.Fatalf("new session kept the description: %q", got)
	}
}

func TestDescribeRefusals(t *testing.T) {
	sid, path, state := descriptionFixture(t)
	for name, tc := range map[string]struct{ panel, description, want string }{
		"no panel":     {"", "Text", "agent panel"},
		"second agent": {"second", "Text", "first agent"},
		"closed panel": {"gone", "Text", "no longer open"},
		"empty text":   {"first", " \n ", "required"},
		"session UUID": {"first", "01a1008a-b13f-74f0-88cb-1b46f", "required"},
	} {
		_, _, err := dispatchBackendCommand(sid, backendCommand{Action: "thread", Panel: tc.panel, Thread: threadCommand{Action: "describe", Description: tc.description}})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	state.Apps[0].SessionID = ""
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "thread", Panel: "first", Thread: threadCommand{Action: "describe", Description: "Text"}}); err == nil {
		t.Error("described a session Libro does not know")
	}
	state.Apps[0].SessionID = "one"
	state.ActiveProject = "thread:qa"
	state.Threads = []Thread{{Managed: true, ID: "thread:qa", Name: "QA", Path: t.TempDir()}}
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "thread", Panel: "first", Thread: threadCommand{Action: "describe", Description: "Text"}}); err == nil {
		t.Error("renamed a QA child")
	}
	if got := state.thread("thread:qa").Name; got != "QA" {
		t.Fatalf("child name = %q", got)
	}
	if got := worktreeTitle(path); got != "" {
		t.Fatalf("refused call saved %q", got)
	}
}

// Between sessions the panel still names the old one, which must not own new text.
func TestDescribeWaitsForTheRunningAgentsSession(t *testing.T) {
	sid, path, _ := descriptionFixture(t)
	if _, err := tm.StartWithEnvironment("first", "sleep 30", path, false, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tm.Stop("first") })
	_, _, err := dispatchBackendCommand(sid, backendCommand{Action: "thread", Panel: "first", Thread: threadCommand{Action: "describe", Description: "Text"}})
	if err == nil || !strings.Contains(err.Error(), "try again") {
		t.Fatalf("error = %v", err)
	}
	if got := worktreeTitle(path); got != "" {
		t.Fatalf("saved %q against the old session", got)
	}
}

func TestDescribeFollowsWorkspaceCleanup(t *testing.T) {
	sid, path, _ := descriptionFixture(t)
	describeOK(t, sid, "first", "Teams page")
	if err := saveWorktreeTitle(path, ""); err != nil {
		t.Fatal(err)
	}
	if got := threadDescriptionSession(applicationPath(path)); got != "" {
		t.Fatalf("cleanup kept the session marker: %q", got)
	}
}

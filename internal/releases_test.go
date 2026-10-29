package libro

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadReleaseStatusComparesTargetsWithHead(t *testing.T) {
	oldDB := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = oldDB })
	createTables()
	_, root, _ := finishFixture(t)
	if _, err := worktreeGit(root, "tag", "-a", "v1.0.0", "-m", "release"); err != nil {
		t.Fatal(err)
	}
	first, err := worktreeGit(root, "rev-parse", "--short", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	commitFinishFile(t, root, "one.txt", "one\n")
	commitFinishFile(t, root, "two.txt", "two\n")
	head, err := worktreeGit(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	targets := []releaseTarget{
		{Name: "Installed", Command: "echo app 1.0.0"},
		{Name: "Stage", Command: "echo " + strings.TrimSpace(first) + "-2-g" + strings.TrimSpace(head)[:7]},
		{Name: "Production", Command: "echo " + strings.TrimSpace(head)},
		{Name: "Unknown", Command: "echo 9.9.9"},
		{Name: "Missing", Command: "echo not installed >&2; exit 1"},
	}
	if err := saveReleaseTargets(root, targets); err != nil {
		t.Fatal(err)
	}

	status := readReleaseStatus("repo", root)
	if status.Error != "" || status.Branch != "main" || status.Tag != "v1.0.0" || status.SinceTag != 2 || len(status.Targets) != 5 {
		t.Fatalf("status: %+v", status)
	}
	installed, stage, production, unknown, missing := status.Targets[0], status.Targets[1], status.Targets[2], status.Targets[3], status.Targets[4]
	if !installed.Known || installed.Version != "v1.0.0" || installed.Behind != 2 || len(installed.Missing) != 2 || installed.Missing[0] != "two.txt" {
		t.Fatalf("installed: %+v", installed)
	}
	if !stage.Known || stage.Behind != 0 || stage.Ahead != 0 {
		t.Fatalf("describe output: %+v", stage)
	}
	if !production.Known || production.Behind != 0 || production.Ahead != 0 {
		t.Fatalf("production: %+v", production)
	}
	if unknown.Known || unknown.Output != "9.9.9" || unknown.Error != "" {
		t.Fatalf("unknown: %+v", unknown)
	}
	if missing.Error != "not installed" {
		t.Fatalf("missing: %+v", missing)
	}
}

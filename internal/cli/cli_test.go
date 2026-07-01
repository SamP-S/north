package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
)

// run executes the north command tree with args in the given working dir,
// returning combined stdout/stderr and the cobra error.
func run(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestCLIInitAndCreate(t *testing.T) {
	dir := t.TempDir()
	if out, err := run(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (%s)", err, out)
	}
	if _, err := board.LocateBoard(dir); err != nil {
		t.Fatalf("board not created: %v", err)
	}
	out, err := run(t, dir, "task", "create", "Add login", "--agent", "opus4.8")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(out, "Created task-1 (draft): Add login") {
		t.Errorf("create output: %q", out)
	}
}

func TestCLIPromoteMoveBoard(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	// Status change on a draft is rejected.
	if _, err := run(t, dir, "task", "move", "task-1", "in_progress"); err == nil {
		t.Error("expected move on draft to fail")
	}
	if out, err := run(t, dir, "task", "promote", "task-1"); err != nil || !strings.Contains(out, "Promoted task-1 (active)") {
		t.Errorf("promote: %q %v", out, err)
	}
	if out, err := run(t, dir, "task", "move", "task-1", "in_progress"); err != nil || !strings.Contains(out, "task-1 → in_progress") {
		t.Errorf("move: %q %v", out, err)
	}
	out, err := run(t, dir, "board")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "in_progress") || !strings.Contains(out, "total") {
		t.Errorf("board output: %q", out)
	}
}

func TestCLIBoardJSONAndPlain(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "promote", "task-1")
	run(t, dir, "task", "move", "task-1", "in_progress")

	out, err := run(t, dir, "board", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"active"`) || !strings.Contains(out, `"in_progress": 1`) ||
		!strings.Contains(out, `"drafts"`) || !strings.Contains(out, `"archive"`) {
		t.Errorf("board --json output: %q", out)
	}

	out, err = run(t, dir, "board", "--plain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "in_progress\t1") || !strings.Contains(out, "drafts\t0") || !strings.Contains(out, "archive\t0") {
		t.Errorf("board --plain output: %q", out)
	}
}

func TestCLIListJSON(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	// Created tasks are drafts; default list (active) is empty.
	out, err := run(t, dir, "task", "list", "--state", "draft", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"id"`) || !strings.Contains(out, "task-1") || !strings.Contains(out, `"state": "draft"`) {
		t.Errorf("json output: %q", out)
	}
}

func TestCLIDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	if out, err := run(t, dir, "task", "delete", "task-1", "--yes"); err != nil || !strings.Contains(out, "Deleted task-1") {
		t.Errorf("delete: %q %v", out, err)
	}
	if out, _ := run(t, dir, "task", "list", "--state", "all", "--plain"); strings.TrimSpace(out) != "" {
		t.Errorf("expected empty list, got %q", out)
	}
}

func TestCLIDeleteOutputFlags(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := run(t, dir, "task", "delete", "task-1", "--yes", "--plain")
	if err != nil || !strings.Contains(out, "id:         task-1") {
		t.Errorf("delete --plain: %q %v", out, err)
	}

	run(t, dir, "task", "create", "y")
	out, err = run(t, dir, "task", "delete", "task-1", "--yes", "--json")
	if err != nil || !strings.Contains(out, `"id"`) || !strings.Contains(out, "task-1") {
		t.Errorf("delete --json: %q %v", out, err)
	}
}

func TestCLISkillShow(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, dir, "skill", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "name: north") || !strings.Contains(out, "north-skill-version:") {
		t.Errorf("skill show output: %q", out)
	}
}

func TestCLISkillInstall(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	out, err := run(t, dir, "skill", "install")
	if err != nil {
		t.Fatalf("skill install: %v (%s)", err, out)
	}
	for _, p := range []string{
		filepath.Join(dir, ".claude", "skills", "north", "SKILL.md"),
		filepath.Join(dir, ".opencode", "skills", "north", "SKILL.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s: %v", p, err)
		}
	}
}

func TestCLINoBoardErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := run(t, dir, "task", "list")
	if err == nil {
		t.Error("expected error when no board present")
	}
}

// runIn is like run but feeds stdin for interactive prompts.
func runIn(t *testing.T, dir, stdin string, args ...string) (string, error) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestCLIView(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "Add login")
	out, err := run(t, dir, "task", "view", "task-1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"state": "draft"`) || !strings.Contains(out, `"status": "ready"`) {
		t.Errorf("view json: %q", out)
	}
	if pl, _ := run(t, dir, "task", "view", "task-1", "--plain"); !strings.Contains(pl, "state:") {
		t.Errorf("view plain: %q", pl)
	}
}

func TestCLIEditBody(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	if _, err := run(t, dir, "task", "edit", "task-1", "--body", "updated body"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, _ := run(t, dir, "task", "view", "task-1", "--plain")
	if !strings.Contains(out, "updated body") {
		t.Errorf("body not applied: %q", out)
	}
}

func TestCLICleanupMessages(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if out, _ := run(t, dir, "cleanup"); !strings.Contains(out, "Nothing to clean up") {
		t.Errorf("empty cleanup: %q", out)
	}
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "promote", "task-1")
	run(t, dir, "task", "move", "task-1", "in_progress")
	run(t, dir, "task", "move", "task-1", "done")
	if out, _ := run(t, dir, "cleanup"); !strings.Contains(out, "Archived 1") {
		t.Errorf("cleanup: %q", out)
	}
}

func TestCLIDeleteDeclined(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := runIn(t, dir, "n\n", "task", "delete", "task-1")
	if err == nil {
		t.Error("declining should exit non-zero")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("expected Aborted: %q", out)
	}
	// Task still exists.
	if _, err := run(t, dir, "task", "view", "task-1"); err != nil {
		t.Errorf("task should survive a declined delete: %v", err)
	}
}

func TestCLIDeleteConfirmed(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := runIn(t, dir, "y\n", "task", "delete", "task-1")
	if err != nil || !strings.Contains(out, "Deleted task-1") {
		t.Errorf("confirmed delete: %q %v", out, err)
	}
}

func TestCLIDeleteWarnsDependents(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "base")
	run(t, dir, "task", "create", "dep", "--depends-on", "task-1")
	out, err := run(t, dir, "task", "delete", "task-1", "--yes")
	if err != nil {
		t.Fatalf("delete: %v (%s)", err, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "task-2") {
		t.Errorf("expected warning about task-2 in output: %q", out)
	}
	// task-2 should still exist (warn-but-allow)
	if _, err := run(t, dir, "task", "view", "task-2"); err != nil {
		t.Errorf("task-2 should still exist after deleting task-1: %v", err)
	}
}

func TestCLIDeleteWarnsJSON(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "base")
	run(t, dir, "task", "create", "dep", "--depends-on", "task-1")
	out, err := run(t, dir, "task", "delete", "task-1", "--yes", "--json")
	if err != nil {
		t.Fatalf("delete --json: %v (%s)", err, out)
	}
	if !strings.Contains(out, `"warnings"`) {
		t.Errorf("expected warnings key in JSON: %q", out)
	}
	if !strings.Contains(out, "task-2") {
		t.Errorf("expected task-2 in warnings: %q", out)
	}
	// task-2 should still exist
	if _, err := run(t, dir, "task", "view", "task-2"); err != nil {
		t.Errorf("task-2 should still exist after deleting task-1: %v", err)
	}
}

func TestCLICreateRejectsInvalidDep(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	_, err := run(t, dir, "task", "create", "x", "--depends-on", "task-99")
	if err == nil {
		t.Error("expected error when depends-on references non-existent task")
	}
}

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

func TestCLIEditBodyFile(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	bodyFile := filepath.Join(dir, "body.txt")
	if err := os.WriteFile(bodyFile, []byte("from a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "task", "edit", "task-1", "--body-file", bodyFile); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, _ := run(t, dir, "task", "view", "task-1", "--plain")
	if !strings.Contains(out, "from a file") {
		t.Errorf("body not applied: %q", out)
	}
	// A missing body file is a clean error, not a panic.
	if _, err := run(t, dir, "task", "edit", "task-1", "--body-file", filepath.Join(dir, "nope.txt")); err == nil {
		t.Error("expected error for missing body file")
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

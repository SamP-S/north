package cli

import (
	"bytes"
	"os"
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

func TestCLIMoveAndBoard(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	if out, err := run(t, dir, "task", "move", "task-1", "ready"); err != nil || !strings.Contains(out, "task-1 → ready") {
		t.Errorf("move: %q %v", out, err)
	}
	out, err := run(t, dir, "board")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ready") || !strings.Contains(out, "total") {
		t.Errorf("board output: %q", out)
	}
}

func TestCLIListJSON(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := run(t, dir, "task", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"id"`) || !strings.Contains(out, "task-1") {
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
	if out, _ := run(t, dir, "task", "list", "--plain"); strings.TrimSpace(out) != "" {
		t.Errorf("expected empty list, got %q", out)
	}
}

func TestCLINoBoardErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := run(t, dir, "task", "list")
	if err == nil {
		t.Error("expected error when no board present")
	}
}

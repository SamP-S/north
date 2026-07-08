package tasks_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/tasks"
)

func gitOut(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "tester"},
		{"config", "user.email", "tester@example.com"},
	} {
		if out, err := gitOut(t, root, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	boardDir, err := board.InitBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	return boardDir
}

func commitMessages(t *testing.T, boardDir string) []string {
	t.Helper()
	out, err := gitOut(t, boardDir, "log", "--format=%s")
	if err != nil {
		return nil // no commits yet
	}
	return strings.Split(out, "\n")
}

func TestAutoCommitCreatesCommit(t *testing.T) {
	boardDir := initRepo(t)
	if _, err := board.WriteConfig(boardDir, board.Config{AutoCommit: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.Create(boardDir, "committed task", "", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range commitMessages(t, boardDir) {
		if strings.HasPrefix(m, "north: create 1") {
			found = true
		}
	}
	if !found {
		t.Errorf("no create commit found: %v", commitMessages(t, boardDir))
	}
}

func TestNoCommitWhenDisabled(t *testing.T) {
	boardDir := initRepo(t)
	if _, _, err := tasks.Create(boardDir, "x", "", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := gitOut(t, boardDir, "rev-parse", "HEAD"); err == nil {
		t.Error("expected no commits, but HEAD is valid")
	}
}

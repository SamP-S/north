package tasks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func newBoard(t *testing.T) string {
	t.Helper()
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return dir
}

func mustCreate(t *testing.T, boardDir, title string) *models.Task {
	t.Helper()
	task, err := tasks.Create(boardDir, title, "", nil, nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return task
}

func isBoardErr(err error, code string) bool {
	be, ok := nerrors.As(err)
	return ok && be.Code() == code
}

func TestNextIDIncrements(t *testing.T) {
	boardDir := newBoard(t)
	if id, _ := board.NextID(boardDir); id != "task-1" {
		t.Errorf("got %s", id)
	}
	mustCreate(t, boardDir, "one")
	mustCreate(t, boardDir, "two")
	if id, _ := board.NextID(boardDir); id != "task-3" {
		t.Errorf("got %s", id)
	}
}

func TestCreateLandsInDraft(t *testing.T) {
	boardDir := newBoard(t)
	task, err := tasks.Create(boardDir, "Add login form", "opus4.8", []string{"auth"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-1" || task.Status != models.Draft {
		t.Errorf("unexpected %+v", task)
	}
	if filepath.Base(filepath.Dir(task.Path)) != "draft" {
		t.Errorf("not in draft: %s", task.Path)
	}
	if filepath.Base(task.Path) != "task-1 - Add-login-form.md" {
		t.Errorf("bad filename: %s", filepath.Base(task.Path))
	}
	if task.CreatedAt == nil || task.UpdatedAt == nil {
		t.Error("timestamps not set")
	}
}

func TestEmptyTitleRejected(t *testing.T) {
	boardDir := newBoard(t)
	_, err := tasks.Create(boardDir, "   ", "", nil, nil, "")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestMoveValidRelocates(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	moved, err := tasks.Move(boardDir, "task-1", "ready")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status != models.Ready || filepath.Base(filepath.Dir(moved.Path)) != "ready" {
		t.Errorf("not moved: %+v", moved)
	}
	if _, err := os.Stat(filepath.Join(boardDir, "draft", "task-1 - x.md")); !os.IsNotExist(err) {
		t.Error("old file still present")
	}
}

func TestMoveIllegalRaises(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	_, err := tasks.Move(boardDir, "task-1", "done")
	if !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestMoveUnknownStatusRaises(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	_, err := tasks.Move(boardDir, "task-1", "nope")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestDoneToReadyReopenAllowed(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	for _, s := range []string{"ready", "in_progress", "done", "ready"} {
		if _, err := tasks.Move(boardDir, "task-1", s); err != nil {
			t.Fatalf("move %s: %v", s, err)
		}
	}
	task, _ := tasks.Get(boardDir, "task-1")
	if task.Status != models.Ready {
		t.Errorf("got %s", task.Status)
	}
}

func TestEditRenamesFile(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "old title")
	title := "new title"
	labels := []string{"a", "b"}
	edited, err := tasks.Edit(boardDir, "task-1", &title, nil, &labels, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(edited.Path) != "task-1 - new-title.md" {
		t.Errorf("not renamed: %s", filepath.Base(edited.Path))
	}
	if _, err := os.Stat(filepath.Join(boardDir, "draft", "task-1 - old-title.md")); !os.IsNotExist(err) {
		t.Error("old file still present")
	}
	if len(edited.Labels) != 2 || edited.Labels[0] != "a" {
		t.Errorf("labels: %v", edited.Labels)
	}
}

func TestDependsOnRoundtrip(t *testing.T) {
	boardDir := newBoard(t)
	if _, err := tasks.Create(boardDir, "x", "", nil, []string{"task-9", "task-3"}, ""); err != nil {
		t.Fatal(err)
	}
	task, _ := tasks.Get(boardDir, "task-1")
	if len(task.DependsOn) != 2 || task.DependsOn[0] != "task-9" || task.DependsOn[1] != "task-3" {
		t.Errorf("deps: %v", task.DependsOn)
	}
}

func TestListFiltersByStatus(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a")
	mustCreate(t, boardDir, "b")
	if _, err := tasks.Move(boardDir, "task-2", "ready"); err != nil {
		t.Fatal(err)
	}
	ready, _ := tasks.List(boardDir, "ready", false)
	if len(ready) != 1 || ready[0].ID != "task-2" {
		t.Errorf("filter failed: %v", ready)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	if err := tasks.Delete(boardDir, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Get(boardDir, "task-1"); !isBoardErr(err, "not_found") {
		t.Fatalf("expected not_found, got %v", err)
	}
}

func TestStatusMirroredInFrontmatter(t *testing.T) {
	boardDir := newBoard(t)
	task := mustCreate(t, boardDir, "x")
	data, _ := os.ReadFile(task.Path)
	if !strings.Contains(string(data), "status: draft") || !strings.Contains(string(data), "id: task-1") {
		t.Errorf("frontmatter mismatch:\n%s", data)
	}
}

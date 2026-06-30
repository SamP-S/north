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

// mustActive creates a task and promotes it onto the active board.
func mustActive(t *testing.T, boardDir, title string) *models.Task {
	t.Helper()
	task := mustCreate(t, boardDir, title)
	active, err := tasks.Promote(boardDir, task.ID)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	return active
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

func TestCreateLandsInDrafts(t *testing.T) {
	boardDir := newBoard(t)
	task, err := tasks.Create(boardDir, "Add login form", "opus4.8", []string{"auth"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-1" || task.State != models.StateDraft || task.Status != models.Ready {
		t.Errorf("unexpected %+v", task)
	}
	if filepath.Base(filepath.Dir(task.Path)) != "drafts" {
		t.Errorf("not in drafts: %s", task.Path)
	}
	if filepath.Base(task.Path) != "task-1-add-login-form.md" {
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

func TestStatusChangeRequiresActive(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x") // draft
	_, err := tasks.SetStatus(boardDir, "task-1", "in_progress")
	if !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict on draft status change, got %v", err)
	}
}

func TestPromoteActivates(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	active, err := tasks.Promote(boardDir, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if active.State != models.StateActive || filepath.Base(filepath.Dir(active.Path)) != "tasks" {
		t.Errorf("not activated: %+v", active)
	}
	if active.Status != models.Ready {
		t.Errorf("promote should preserve status ready, got %s", active.Status)
	}
	if _, err := os.Stat(filepath.Join(boardDir, "drafts", "task-1-x.md")); !os.IsNotExist(err) {
		t.Error("old draft file still present")
	}
}

func TestStatusTransitionsInPlace(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	moved, err := tasks.SetStatus(boardDir, "task-1", "in_progress")
	if err != nil {
		t.Fatal(err)
	}
	// Status change must NOT move the file out of tasks/.
	if moved.Status != models.InProgress || filepath.Base(filepath.Dir(moved.Path)) != "tasks" {
		t.Errorf("status change misbehaved: %+v", moved)
	}
	if _, err := tasks.SetStatus(boardDir, "task-1", "done"); err != nil {
		t.Fatalf("in_progress→done: %v", err)
	}
}

func TestStatusIllegalRejected(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	_, err := tasks.SetStatus(boardDir, "task-1", "done") // ready→done illegal
	if !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestStatusUnknownRejected(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	_, err := tasks.SetStatus(boardDir, "task-1", "nope")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestDoneToReadyReopen(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	for _, s := range []string{"in_progress", "done", "ready"} {
		if _, err := tasks.SetStatus(boardDir, "task-1", s); err != nil {
			t.Fatalf("status %s: %v", s, err)
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
	if filepath.Base(edited.Path) != "task-1-new-title.md" {
		t.Errorf("not renamed: %s", filepath.Base(edited.Path))
	}
	if _, err := os.Stat(filepath.Join(boardDir, "drafts", "task-1-old-title.md")); !os.IsNotExist(err) {
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

func TestListFiltersByState(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a") // draft
	mustActive(t, boardDir, "b") // active (task-2)
	active, _ := tasks.List(boardDir, []models.TaskState{models.StateActive}, "")
	if len(active) != 1 || active[0].ID != "task-2" {
		t.Errorf("active filter: %v", active)
	}
	drafts, _ := tasks.List(boardDir, []models.TaskState{models.StateDraft}, "")
	if len(drafts) != 1 || drafts[0].ID != "task-1" {
		t.Errorf("draft filter: %v", drafts)
	}
	all, _ := tasks.List(boardDir, nil, "")
	if len(all) != 2 {
		t.Errorf("all: %v", all)
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
	if !strings.Contains(string(data), "status: ready") || !strings.Contains(string(data), "id: task-1") {
		t.Errorf("frontmatter mismatch:\n%s", data)
	}
}

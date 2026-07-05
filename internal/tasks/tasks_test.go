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

// mustActive creates a task and moves it onto the active board.
func mustActive(t *testing.T, boardDir, title string) *models.Task {
	t.Helper()
	task := mustCreate(t, boardDir, title)
	active, err := tasks.SetState(boardDir, task.ID, "active")
	if err != nil {
		t.Fatalf("set state active: %v", err)
	}
	return active
}

// list is a test helper over the tolerant snapshot load.
func list(t *testing.T, boardDir string, states []models.TaskState, status string) []*models.Task {
	t.Helper()
	snap, err := tasks.Load(boardDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return snap.Filter(states, status)
}

func isBoardErr(err error, code string) bool {
	be, ok := nerrors.As(err)
	return ok && be.Code() == code
}

func TestNextIDIncrements(t *testing.T) {
	boardDir := newBoard(t)
	if id, _ := board.NextID(boardDir); id != "1" {
		t.Errorf("got %s", id)
	}
	mustCreate(t, boardDir, "one")
	mustCreate(t, boardDir, "two")
	if id, _ := board.NextID(boardDir); id != "3" {
		t.Errorf("got %s", id)
	}
}

func TestCreateLandsInDrafts(t *testing.T) {
	boardDir := newBoard(t)
	task, err := tasks.Create(boardDir, "Add login form", "opus4.8", []string{"auth"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "1" || task.State != models.StateDraft || task.Status != models.Ready {
		t.Errorf("unexpected %+v", task)
	}
	if filepath.Base(filepath.Dir(task.Path)) != "drafts" {
		t.Errorf("not in drafts: %s", task.Path)
	}
	if filepath.Base(task.Path) != "1-add-login-form.md" {
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

func TestStatusChangeAnyState(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x") // draft
	task, err := tasks.SetStatus(boardDir, "1", "in_progress")
	if err != nil {
		t.Fatalf("status change on draft: %v", err)
	}
	if task.Status != models.InProgress || task.State != models.StateDraft {
		t.Errorf("got status=%s state=%s", task.Status, task.State)
	}
	// The status survives activation.
	task, err = tasks.SetState(boardDir, "1", "active")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != models.InProgress {
		t.Errorf("status lost on activation: %s", task.Status)
	}
}

func TestStatusFreeform(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	// Any status → any status in one step, no required path.
	for _, s := range []string{"done", "ready", "failed", "blocked", "in_progress", "ready"} {
		if _, err := tasks.SetStatus(boardDir, "1", s); err != nil {
			t.Fatalf("status %s: %v", s, err)
		}
	}
	task, _ := tasks.Get(boardDir, "1")
	if task.Status != models.Ready {
		t.Errorf("got %s", task.Status)
	}
}

func TestStatusChangeStaysInPlace(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	moved, err := tasks.SetStatus(boardDir, "1", "in_progress")
	if err != nil {
		t.Fatal(err)
	}
	// Status change must NOT move the file out of tasks/.
	if moved.Status != models.InProgress || filepath.Base(filepath.Dir(moved.Path)) != "tasks" {
		t.Errorf("status change misbehaved: %+v", moved)
	}
}

func TestStatusUnknownRejected(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	_, err := tasks.SetStatus(boardDir, "1", "nope")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestEditRenamesFile(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "old title")
	title := "new title"
	labels := []string{"a", "b"}
	edited, err := tasks.Edit(boardDir, "1", tasks.EditOpts{Title: &title, Labels: &labels})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(edited.Path) != "1-new-title.md" {
		t.Errorf("not renamed: %s", filepath.Base(edited.Path))
	}
	if _, err := os.Stat(filepath.Join(boardDir, "drafts", "1-old-title.md")); !os.IsNotExist(err) {
		t.Error("old file still present")
	}
	if len(edited.Labels) != 2 || edited.Labels[0] != "a" {
		t.Errorf("labels: %v", edited.Labels)
	}
}

func TestEditAppendBody(t *testing.T) {
	boardDir := newBoard(t)
	task, err := tasks.Create(boardDir, "x", "", nil, nil, "first line")
	if err != nil {
		t.Fatal(err)
	}
	note := "appended note"
	edited, err := tasks.Edit(boardDir, task.ID, tasks.EditOpts{AppendBody: &note})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Body != "first line\n\nappended note" {
		t.Errorf("append body: %q", edited.Body)
	}
	// Appending to an empty body just sets it.
	task2 := mustCreate(t, boardDir, "y")
	edited2, err := tasks.Edit(boardDir, task2.ID, tasks.EditOpts{AppendBody: &note})
	if err != nil {
		t.Fatal(err)
	}
	if edited2.Body != "appended note" {
		t.Errorf("append to empty body: %q", edited2.Body)
	}
}

func TestDependsOnRoundtrip(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "dep-a") // 1
	mustCreate(t, boardDir, "dep-b") // 2
	if _, err := tasks.Create(boardDir, "x", "", nil, []string{"1", "2"}, ""); err != nil {
		t.Fatal(err)
	}
	task, _ := tasks.Get(boardDir, "3")
	if len(task.DependsOn) != 2 || task.DependsOn[0] != "1" || task.DependsOn[1] != "2" {
		t.Errorf("deps: %v", task.DependsOn)
	}
}

func TestListFiltersByState(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a") // draft
	mustActive(t, boardDir, "b") // active (2)
	active := list(t, boardDir, []models.TaskState{models.StateActive}, "")
	if len(active) != 1 || active[0].ID != "2" {
		t.Errorf("active filter: %v", active)
	}
	drafts := list(t, boardDir, []models.TaskState{models.StateDraft}, "")
	if len(drafts) != 1 || drafts[0].ID != "1" {
		t.Errorf("draft filter: %v", drafts)
	}
	if all := list(t, boardDir, nil, ""); len(all) != 2 {
		t.Errorf("all: %v", all)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	if err := tasks.Delete(boardDir, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Get(boardDir, "1"); !isBoardErr(err, "not_found") {
		t.Fatalf("expected not_found, got %v", err)
	}
}

func TestFrontmatterShape(t *testing.T) {
	boardDir := newBoard(t)
	task := mustCreate(t, boardDir, "x")
	data, _ := os.ReadFile(task.Path)
	// The id must be a quoted string so YAML never coerces it to an int.
	if !strings.Contains(string(data), `id: "1"`) || !strings.Contains(string(data), "status: ready") {
		t.Errorf("frontmatter mismatch:\n%s", data)
	}
}

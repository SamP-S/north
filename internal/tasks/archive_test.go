package tasks_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func TestStateFreeform(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	// Any state → any state in one step, including archive → active directly.
	for _, s := range []string{"active", "archive", "active", "draft", "archive", "draft"} {
		task, err := tasks.SetState(boardDir, "1", s)
		if err != nil {
			t.Fatalf("state %s: %v", s, err)
		}
		if string(task.State) != s {
			t.Errorf("expected state %s, got %s", s, task.State)
		}
	}
}

func TestArchiveMovesFile(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	archived, err := tasks.SetState(boardDir, "1", "archive")
	if err != nil {
		t.Fatal(err)
	}
	if archived.State != models.StateArchive || filepath.Base(filepath.Dir(archived.Path)) != "archive" {
		t.Errorf("not archived: %+v", archived)
	}
	if active := list(t, boardDir, []models.TaskState{models.StateActive}, ""); len(active) != 0 {
		t.Errorf("active not empty: %v", active)
	}
	if arc := list(t, boardDir, []models.TaskState{models.StateArchive}, ""); len(arc) != 1 {
		t.Errorf("archive list: %v", arc)
	}
}

func TestStateSameIsNoOp(t *testing.T) {
	boardDir := newBoard(t)
	task := mustCreate(t, boardDir, "x")
	before := task.UpdatedAt
	same, err := tasks.SetState(boardDir, "1", "draft")
	if err != nil {
		t.Fatalf("same-state move should be a no-op, got %v", err)
	}
	// Compare at stored (RFC3339, second) precision — the reloaded task loses
	// the in-memory nanoseconds.
	if same.UpdatedAt == nil || before == nil ||
		same.UpdatedAt.Format(time.RFC3339) != before.Format(time.RFC3339) {
		t.Errorf("no-op state move must not bump updated_at")
	}
}

func TestStatePreservesStatus(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	if _, err := tasks.SetStatus(boardDir, "1", "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetState(boardDir, "1", "archive"); err != nil {
		t.Fatal(err)
	}
	task, _ := tasks.Get(boardDir, "1")
	if task.Status != models.Done {
		t.Errorf("state change should preserve status done, got %s", task.Status)
	}
}

func TestStatusChangeWhenArchived(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	if _, err := tasks.SetState(boardDir, "1", "archive"); err != nil {
		t.Fatal(err)
	}
	// Freeform: status is editable in any state, including archive.
	task, err := tasks.SetStatus(boardDir, "1", "in_progress")
	if err != nil {
		t.Fatalf("status change on archived task: %v", err)
	}
	if task.Status != models.InProgress || task.State != models.StateArchive {
		t.Errorf("got status=%s state=%s", task.Status, task.State)
	}
}

func TestStateUnknownRejected(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	_, err := tasks.SetState(boardDir, "1", "limbo")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestCleanupArchivesActiveDone(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "done one")    // 1
	mustActive(t, boardDir, "still going") // 2
	if _, err := tasks.SetStatus(boardDir, "1", "done"); err != nil {
		t.Fatal(err)
	}
	archived, err := tasks.Cleanup(boardDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != "1" {
		t.Errorf("cleanup archived: %v", archived)
	}
	active := list(t, boardDir, []models.TaskState{models.StateActive}, "")
	if len(active) != 1 || active[0].ID != "2" {
		t.Errorf("active: %v", active)
	}
}

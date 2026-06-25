package tasks_test

import (
	"path/filepath"
	"testing"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func TestArchiveAndRestore(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	archived, err := tasks.Archive(boardDir, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if archived.State != models.StateArchive || filepath.Base(filepath.Dir(archived.Path)) != "archive" {
		t.Errorf("not archived: %+v", archived)
	}
	// Excluded from active list, present in archive list.
	if active, _ := tasks.List(boardDir, []models.TaskState{models.StateActive}, ""); len(active) != 0 {
		t.Errorf("active not empty: %v", active)
	}
	if arc, _ := tasks.List(boardDir, []models.TaskState{models.StateArchive}, ""); len(arc) != 1 {
		t.Errorf("archive list: %v", arc)
	}
	// Restore brings it back to active.
	restored, err := tasks.Restore(boardDir, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if restored.State != models.StateActive {
		t.Errorf("not restored: %+v", restored)
	}
}

func TestArchivePreservesStatus(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	for _, s := range []string{"in_progress", "done"} {
		if _, err := tasks.SetStatus(boardDir, "task-1", s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tasks.Archive(boardDir, "task-1"); err != nil {
		t.Fatal(err)
	}
	task, _ := tasks.Get(boardDir, "task-1")
	if task.Status != models.Done {
		t.Errorf("archive should preserve status done, got %s", task.Status)
	}
}

func TestCannotChangeStatusWhenArchived(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	if _, err := tasks.Archive(boardDir, "task-1"); err != nil {
		t.Fatal(err)
	}
	_, err := tasks.SetStatus(boardDir, "task-1", "in_progress")
	if !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestDemote(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x")
	demoted, err := tasks.Demote(boardDir, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if demoted.State != models.StateDraft {
		t.Errorf("not demoted: %+v", demoted)
	}
}

func TestPromoteOnlyDraft(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "x") // already active
	_, err := tasks.Promote(boardDir, "task-1")
	if !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict promoting active task, got %v", err)
	}
}

func TestCleanupArchivesActiveDone(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "done one")    // task-1
	mustActive(t, boardDir, "still going") // task-2
	for _, s := range []string{"in_progress", "done"} {
		if _, err := tasks.SetStatus(boardDir, "task-1", s); err != nil {
			t.Fatal(err)
		}
	}
	archived, err := tasks.Cleanup(boardDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != "task-1" {
		t.Errorf("cleanup archived: %v", archived)
	}
	active, _ := tasks.List(boardDir, []models.TaskState{models.StateActive}, "")
	if len(active) != 1 || active[0].ID != "task-2" {
		t.Errorf("active: %v", active)
	}
}

package tasks_test

import (
	"path/filepath"
	"testing"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func toDone(t *testing.T, boardDir, id string) {
	t.Helper()
	for _, s := range []string{"ready", "in_progress", "done"} {
		if _, err := tasks.Move(boardDir, id, s); err != nil {
			t.Fatalf("move %s: %v", s, err)
		}
	}
}

func TestArchiveMovesOffActiveBoard(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	archived, err := tasks.Archive(boardDir, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Archived || filepath.Base(filepath.Dir(archived.Path)) != "archive" {
		t.Errorf("not archived: %+v", archived)
	}
	active, _ := tasks.List(boardDir, "", false)
	if len(active) != 0 {
		t.Errorf("active not empty: %v", active)
	}
	all, _ := tasks.List(boardDir, "", true)
	if len(all) != 1 || all[0].ID != "task-1" {
		t.Errorf("archived list: %v", all)
	}
}

func TestArchivedStatusReadFromFrontmatter(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	toDone(t, boardDir, "task-1")
	if _, err := tasks.Archive(boardDir, "task-1"); err != nil {
		t.Fatal(err)
	}
	task, _ := tasks.Get(boardDir, "task-1")
	if task.Status != models.Done {
		t.Errorf("got %s", task.Status)
	}
}

func TestCannotMoveArchived(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	if _, err := tasks.Archive(boardDir, "task-1"); err != nil {
		t.Fatal(err)
	}
	_, err := tasks.Move(boardDir, "task-1", "ready")
	if !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestCleanupArchivesDoneOnly(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "done one")
	mustCreate(t, boardDir, "still draft")
	toDone(t, boardDir, "task-1")
	archived, err := tasks.Cleanup(boardDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != "task-1" {
		t.Errorf("cleanup archived: %v", archived)
	}
	active, _ := tasks.List(boardDir, "", false)
	if len(active) != 1 || active[0].ID != "task-2" {
		t.Errorf("active: %v", active)
	}
}

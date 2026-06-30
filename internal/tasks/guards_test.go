package tasks_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func TestStateGuards(t *testing.T) {
	t.Run("restore rejects a draft", func(t *testing.T) {
		boardDir := newBoard(t)
		mustCreate(t, boardDir, "x") // draft
		if _, err := tasks.Restore(boardDir, "task-1"); !isBoardErr(err, "conflict") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("demote rejects an archived task", func(t *testing.T) {
		boardDir := newBoard(t)
		mustActive(t, boardDir, "x")
		if _, err := tasks.Archive(boardDir, "task-1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Demote(boardDir, "task-1"); !isBoardErr(err, "conflict") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("archive rejects an already-archived task", func(t *testing.T) {
		boardDir := newBoard(t)
		mustActive(t, boardDir, "x")
		if _, err := tasks.Archive(boardDir, "task-1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Archive(boardDir, "task-1"); !isBoardErr(err, "conflict") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("operations on a missing task are not_found", func(t *testing.T) {
		boardDir := newBoard(t)
		for _, op := range []func(string, string) (*models.Task, error){
			tasks.Promote, tasks.Demote, tasks.Archive, tasks.Restore,
		} {
			if _, err := op(boardDir, "task-99"); !isBoardErr(err, "not_found") {
				t.Errorf("expected not_found, got %v", err)
			}
		}
	})
}

func TestCleanupRespectsOlderThan(t *testing.T) {
	boardDir := newBoard(t)
	old := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Format(time.RFC3339)
	writeRaw(t, boardDir, "tasks", "task-1-old.md", doneTask("task-1", "old", old))
	writeRaw(t, boardDir, "tasks", "task-2-fresh.md", doneTask("task-2", "fresh", recent))

	archived, err := tasks.Cleanup(boardDir, 5) // only tasks older than 5 days
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != "task-1" {
		t.Fatalf("cleanup archived %v, want just task-1", archived)
	}
	// The fresh done task remains active.
	if c, _ := tasks.StateCount(boardDir, models.StateActive); c != 1 {
		t.Errorf("active count = %d, want 1", c)
	}
}

func TestNextIDCountsArchiveAndReusesTop(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a") // task-1
	mustActive(t, boardDir, "b") // task-2 (active)
	if _, err := tasks.Archive(boardDir, "task-2"); err != nil {
		t.Fatal(err)
	}
	// Archived task-2 still reserves its id even after task-1 is deleted.
	if err := tasks.Delete(boardDir, "task-1"); err != nil {
		t.Fatal(err)
	}
	if id, _ := board.NextID(boardDir); id != "task-3" {
		t.Errorf("archive should reserve ids: NextID=%s want task-3", id)
	}
	// Deleting the highest id frees it for reuse (derived, no stored counter).
	boardDir2 := newBoard(t)
	mustCreate(t, boardDir2, "a") // task-1
	if err := tasks.Delete(boardDir2, "task-1"); err != nil {
		t.Fatal(err)
	}
	if id, _ := board.NextID(boardDir2); id != "task-1" {
		t.Errorf("top delete should reuse: NextID=%s want task-1", id)
	}
}

func doneTask(id, title, updated string) string {
	return fmt.Sprintf("---\nid: %s\ntitle: %s\nstatus: done\nagent: \"\"\nlabels: []\ndepends_on: []\ncreated_at: %q\nupdated_at: %q\n---\n",
		id, title, updated, updated)
}

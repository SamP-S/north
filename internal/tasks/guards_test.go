package tasks_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func TestOpsOnMissingTaskAreNotFound(t *testing.T) {
	boardDir := newBoard(t)
	if _, err := tasks.SetState(boardDir, "99", "active"); !isBoardErr(err, "not_found") {
		t.Errorf("SetState: expected not_found, got %v", err)
	}
	if _, _, err := tasks.SetStatus(boardDir, "99", "ready"); !isBoardErr(err, "not_found") {
		t.Errorf("SetStatus: expected not_found, got %v", err)
	}
	if _, _, err := tasks.Edit(boardDir, "99", tasks.EditOpts{}); !isBoardErr(err, "not_found") {
		t.Errorf("Edit: expected not_found, got %v", err)
	}
	if _, err := tasks.Delete(boardDir, "99"); !isBoardErr(err, "not_found") {
		t.Errorf("Delete: expected not_found, got %v", err)
	}
}

func TestCleanupRespectsOlderThan(t *testing.T) {
	boardDir := newBoard(t)
	old := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Format(time.RFC3339)
	writeRaw(t, boardDir, "tasks", "1-old.md", doneTask("1", "old", old))
	writeRaw(t, boardDir, "tasks", "2-fresh.md", doneTask("2", "fresh", recent))

	archived, err := tasks.Cleanup(boardDir, 5) // only tasks older than 5 days
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != "1" {
		t.Fatalf("cleanup archived %v, want just 1", archived)
	}
	// The fresh done task remains active.
	snap, _ := tasks.Load(boardDir)
	if c := snap.StateCount(models.StateActive); c != 1 {
		t.Errorf("active count = %d, want 1", c)
	}
}

func TestNextIDCountsArchiveAndReusesTop(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a") // 1
	mustActive(t, boardDir, "b") // 2 (active)
	if _, err := tasks.SetState(boardDir, "2", "archive"); err != nil {
		t.Fatal(err)
	}
	// Archived 2 still reserves its id even after 1 is deleted.
	if _, err := tasks.Delete(boardDir, "1"); err != nil {
		t.Fatal(err)
	}
	if id, _ := board.NextID(boardDir); id != "3" {
		t.Errorf("archive should reserve ids: NextID=%s want 3", id)
	}
	// Deleting the highest id frees it for reuse (derived, no stored counter).
	boardDir2 := newBoard(t)
	mustCreate(t, boardDir2, "a") // 1
	if _, err := tasks.Delete(boardDir2, "1"); err != nil {
		t.Fatal(err)
	}
	if id, _ := board.NextID(boardDir2); id != "1" {
		t.Errorf("top delete should reuse: NextID=%s want 1", id)
	}
}

func doneTask(id, title, updated string) string {
	return fmt.Sprintf("---\nid: %q\ntitle: %s\nstatus: done\nagent: \"\"\nlabels: []\ndepends_on: []\ncreated_at: %q\nupdated_at: %q\n---\n",
		id, title, updated, updated)
}

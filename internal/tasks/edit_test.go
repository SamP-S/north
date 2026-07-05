package tasks_test

import (
	"testing"

	"github.com/SamP-S/north/internal/tasks"
)

func TestEditClearsVsLeavesAlone(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "dep") // 1; will be used as a dep
	// Create 2 depending on 1.
	if _, err := tasks.Create(boardDir, "x", "ag", []string{"a", "b"}, []string{"1"}, "body"); err != nil {
		t.Fatal(err)
	}
	// Pass an empty (non-nil) labels slice to CLEAR; leave deps nil to KEEP.
	empty := []string{}
	edited, err := tasks.Edit(boardDir, "2", tasks.EditOpts{Labels: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if len(edited.Labels) != 0 {
		t.Errorf("labels not cleared: %v", edited.Labels)
	}
	if len(edited.DependsOn) != 1 || edited.DependsOn[0] != "1" {
		t.Errorf("deps should be untouched: %v", edited.DependsOn)
	}
	if edited.Assignee != "ag" || edited.Body != "body" {
		t.Errorf("agent/body should be untouched: agent=%q body=%q", edited.Assignee, edited.Body)
	}
}

func TestEditEmptyTitleRejected(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "x")
	blank := "   "
	if _, err := tasks.Edit(boardDir, "1", tasks.EditOpts{Title: &blank}); !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestEditBumpsUpdatedAt(t *testing.T) {
	boardDir := newBoard(t)
	created := mustCreate(t, boardDir, "x")
	body := "new"
	edited, err := tasks.Edit(boardDir, "1", tasks.EditOpts{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if edited.UpdatedAt == nil || created.UpdatedAt == nil || edited.UpdatedAt.Before(*created.UpdatedAt) {
		t.Error("updated_at not bumped")
	}
}

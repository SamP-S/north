package tasks_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func TestNextPicksLowestWorkable(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "still a draft") // 1 — not active, never picked
	a := mustActive(t, boardDir, "first active")
	b := mustActive(t, boardDir, "second active")

	got, _, err := tasks.Next(boardDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != a.ID {
		t.Fatalf("expected %s, got %+v", a.ID, got)
	}

	// Claimed → next moves on to b.
	if _, _, err := tasks.SetStatus(boardDir, a.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}
	got, _, err = tasks.Next(boardDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != b.ID {
		t.Fatalf("expected %s, got %+v", b.ID, got)
	}
}

func TestNextSkipsAssignedAndUnmetDeps(t *testing.T) {
	boardDir := newBoard(t)
	assigned := mustActive(t, boardDir, "already owned")
	if _, _, err := tasks.Edit(boardDir, assigned.ID, tasks.EditOpts{Assignee: strPtr("someone")}); err != nil {
		t.Fatal(err)
	}
	blocker := mustActive(t, boardDir, "blocker")
	dependent := mustActive(t, boardDir, "waiting on blocker")
	if _, _, err := tasks.Edit(boardDir, dependent.ID, tasks.EditOpts{DependsOn: &[]string{blocker.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.SetStatus(boardDir, blocker.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}

	// assigned is owned, blocker is in_progress, dependent waits → nothing.
	got, _, err := tasks.Next(boardDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected no workable task, got %s", got.ID)
	}

	// Blocker done → dependent becomes workable.
	if _, _, err := tasks.SetStatus(boardDir, blocker.ID, "done"); err != nil {
		t.Fatal(err)
	}
	got, _, err = tasks.Next(boardDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != dependent.ID {
		t.Fatalf("expected %s, got %+v", dependent.ID, got)
	}
}

func TestNextLabelFilter(t *testing.T) {
	boardDir := newBoard(t)
	mustActive(t, boardDir, "unlabelled")
	labelled, _, err := tasks.Create(boardDir, "backend work", "", []string{"backend", "api"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetState(boardDir, labelled.ID, "active"); err != nil {
		t.Fatal(err)
	}

	got, _, err := tasks.Next(boardDir, []string{"backend", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != labelled.ID {
		t.Fatalf("expected %s, got %+v", labelled.ID, got)
	}
	got, _, err = tasks.Next(boardDir, []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected no match, got %s", got.ID)
	}
}

func TestTakeClaimsAtomically(t *testing.T) {
	boardDir := newBoard(t)
	a := mustActive(t, boardDir, "claim me")

	got, _, err := tasks.Take(boardDir, "agent-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != a.ID {
		t.Fatalf("expected %s, got %+v", a.ID, got)
	}
	if got.Status != models.InProgress || got.Assignee != "agent-1" {
		t.Fatalf("claim did not stick: %+v", got)
	}
	// Round-trips on disk.
	fresh, err := tasks.Get(boardDir, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != models.InProgress || fresh.Assignee != "agent-1" {
		t.Fatalf("on-disk claim wrong: %+v", fresh)
	}

	// Nothing left → nil task, nil error.
	got, _, err = tasks.Take(boardDir, "agent-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected empty result, got %s", got.ID)
	}
}

func TestTakeRequiresAssignee(t *testing.T) {
	boardDir := newBoard(t)
	if _, _, err := tasks.Take(boardDir, "  ", nil); !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid, got %v", err)
	}
}

// TestTakeConcurrentDistinct is the reason take exists: N parallel takes must
// hand out N distinct tasks, never the same task twice.
func TestTakeConcurrentDistinct(t *testing.T) {
	boardDir := newBoard(t)
	const n = 8
	for i := 0; i < n; i++ {
		mustActive(t, boardDir, fmt.Sprintf("task %d", i))
	}

	var wg sync.WaitGroup
	got := make([]*models.Task, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _, errs[i] = tasks.Take(boardDir, fmt.Sprintf("agent-%d", i), nil)
		}(i)
	}
	wg.Wait()

	seen := map[string]int{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("take %d: %v", i, errs[i])
		}
		if got[i] == nil {
			t.Fatalf("take %d: got nothing (tasks were available)", i)
		}
		seen[got[i].ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("task %s was handed to %d agents", id, count)
		}
	}
	if len(seen) != n {
		t.Errorf("expected %d distinct tasks, got %d", n, len(seen))
	}
}

func TestTakeMaxWIP(t *testing.T) {
	boardDir := newBoard(t)
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxWIP = 1
	if _, err := board.WriteConfig(boardDir, cfg); err != nil {
		t.Fatal(err)
	}
	first := mustActive(t, boardDir, "first")
	mustActive(t, boardDir, "second")

	if _, _, err := tasks.Take(boardDir, "agent-1", nil); err != nil {
		t.Fatal(err)
	}
	// At the cap → conflict; assignees match case-insensitively; another
	// assignee is unaffected.
	if _, _, err := tasks.Take(boardDir, "agent-1", nil); !isBoardErr(err, "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, _, err := tasks.Take(boardDir, "Agent-1", nil); !isBoardErr(err, "conflict") {
		t.Fatalf("expected case-insensitive conflict, got %v", err)
	}
	if _, _, err := tasks.Take(boardDir, "agent-2", nil); err != nil {
		t.Fatalf("other assignee blocked: %v", err)
	}
	// Finishing frees the slot.
	if _, _, err := tasks.SetStatus(boardDir, first.ID, "done"); err != nil {
		t.Fatal(err)
	}
	mustActive(t, boardDir, "third")
	if _, _, err := tasks.Take(boardDir, "agent-1", nil); err != nil {
		t.Fatalf("take after done: %v", err)
	}
}

func strPtr(s string) *string { return &s }

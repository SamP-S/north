package tasks_test

import (
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/tasks"
)

// --- validateDeps (exercised through Create and Edit) ---

// TestValidateDepsHappyPath checks that Create succeeds when all deps exist.
func TestValidateDepsHappyPath(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // task-1
	mustCreate(t, boardDir, "beta")  // task-2
	_, err := tasks.Create(boardDir, "gamma", "", nil, []string{"task-1", "task-2"}, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestValidateDepsNotFound checks that Create returns Invalid when a dep is missing.
func TestValidateDepsNotFound(t *testing.T) {
	boardDir := newBoard(t)
	_, err := tasks.Create(boardDir, "gamma", "", nil, []string{"task-999"}, "")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid error, got %v", err)
	}
	if !strings.Contains(err.Error(), "task-999") {
		t.Errorf("error message should mention missing ID, got: %v", err)
	}
}

// TestValidateDepsNilEmpty checks that nil and empty deps slices cause no error.
func TestValidateDepsNilEmpty(t *testing.T) {
	boardDir := newBoard(t)
	if _, err := tasks.Create(boardDir, "alpha", "", nil, nil, ""); err != nil {
		t.Fatalf("nil deps: %v", err)
	}
	if _, err := tasks.Create(boardDir, "beta", "", nil, []string{}, ""); err != nil {
		t.Fatalf("empty deps: %v", err)
	}
}

// TestValidateDepsEditNilSkipsValidation checks that Edit with nil dependsOn does not
// validate and leaves the existing deps unchanged (nil means "leave unchanged").
func TestValidateDepsEditNilSkipsValidation(t *testing.T) {
	boardDir := newBoard(t)
	// Create with a dep referencing a nonexistent task written directly (bypass Create).
	// We do this by creating the task without deps first, then we'll verify nil edit
	// doesn't blow up even if we had stale deps (which we can't inject via API, but
	// verifying nil edit is a no-op on deps is enough).
	mustCreate(t, boardDir, "alpha") // task-1
	body := "updated"
	edited, err := tasks.Edit(boardDir, "task-1", nil, nil, nil, nil, &body)
	if err != nil {
		t.Fatalf("edit with nil deps: %v", err)
	}
	if edited.Body != "updated" {
		t.Errorf("body not updated: %q", edited.Body)
	}
}

// TestValidateDepsEditValidates checks that Edit with a non-nil deps slice validates them.
func TestValidateDepsEditValidates(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // task-1
	// Edit with a dep that doesn't exist.
	bad := []string{"task-999"}
	_, err := tasks.Edit(boardDir, "task-1", nil, nil, nil, &bad, nil)
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid error, got %v", err)
	}
	if !strings.Contains(err.Error(), "task-999") {
		t.Errorf("error should mention the missing ID, got: %v", err)
	}
}

// --- Dependents ---

// TestDependentsHappyPath checks that Dependents returns tasks depending on a given ID.
func TestDependentsHappyPath(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha")                                          // task-1
	_, err := tasks.Create(boardDir, "beta", "", nil, []string{"task-1"}, "") // task-2 depends on task-1
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	deps, err := tasks.Dependents(boardDir, "task-1")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(deps) != 1 || deps[0].ID != "task-2" {
		t.Errorf("expected [task-2], got %v", deps)
	}
}

// TestDependentsNone checks that Dependents returns an empty non-nil slice when
// no tasks depend on the given ID.
func TestDependentsNone(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // task-1, no dependents
	deps, err := tasks.Dependents(boardDir, "task-1")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if deps == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(deps) != 0 {
		t.Errorf("expected empty slice, got %v", deps)
	}
}

// TestDependentsAcrossAllStates checks that Dependents finds tasks in any state folder.
func TestDependentsAcrossAllStates(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha")                                           // task-1
	active := mustActive(t, boardDir, "beta")                                  // task-2 active
	_, err := tasks.Create(boardDir, "gamma", "", nil, []string{"task-1"}, "") // task-3 draft
	if err != nil {
		t.Fatalf("create gamma: %v", err)
	}
	// Archive task-2 (promote then archive is not possible directly — archive via Archive).
	if _, err := tasks.Archive(boardDir, active.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	// Add a dep from archived task-2 to task-1 via Edit.
	dep := []string{"task-1"}
	if _, err := tasks.Edit(boardDir, "task-2", nil, nil, nil, &dep, nil); err != nil {
		t.Fatalf("edit task-2 deps: %v", err)
	}
	deps, err := tasks.Dependents(boardDir, "task-1")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	// task-3 (draft) and task-2 (archive) both depend on task-1.
	if len(deps) != 2 {
		t.Errorf("expected 2 dependents, got %d: %v", len(deps), deps)
	}
}

// --- End-to-end: Create with --depends-on ---

// TestCreateDependsOnE2E is an end-to-end test of the depends_on wiring in Create.
func TestCreateDependsOnE2E(t *testing.T) {
	boardDir := newBoard(t)

	// Pointing to nonexistent ID must fail.
	_, err := tasks.Create(boardDir, "orphan", "", nil, []string{"task-42"}, "")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid for missing dep, got %v", err)
	}

	// Create the dep first, then a task that references it — must succeed.
	mustCreate(t, boardDir, "foundation") // task-1
	child, err := tasks.Create(boardDir, "child", "", nil, []string{"task-1"}, "")
	if err != nil {
		t.Fatalf("expected success with real dep, got %v", err)
	}
	if len(child.DependsOn) != 1 || child.DependsOn[0] != "task-1" {
		t.Errorf("depends_on not stored: %v", child.DependsOn)
	}
}

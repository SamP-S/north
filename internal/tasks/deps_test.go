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
	mustCreate(t, boardDir, "alpha") // 1
	mustCreate(t, boardDir, "beta")  // 2
	_, err := tasks.Create(boardDir, "gamma", "", nil, []string{"1", "2"}, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestValidateDepsNotFound checks that Create returns Invalid when a dep is missing.
func TestValidateDepsNotFound(t *testing.T) {
	boardDir := newBoard(t)
	_, err := tasks.Create(boardDir, "gamma", "", nil, []string{"999"}, "")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid error, got %v", err)
	}
	if !strings.Contains(err.Error(), "999") {
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

// TestValidateDepsEditNilSkipsValidation checks that Edit with nil DependsOn
// leaves the existing deps unchanged (nil means "leave unchanged").
func TestValidateDepsEditNilSkipsValidation(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // 1
	body := "updated"
	edited, err := tasks.Edit(boardDir, "1", tasks.EditOpts{Body: &body})
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
	mustCreate(t, boardDir, "alpha") // 1
	bad := []string{"999"}
	_, err := tasks.Edit(boardDir, "1", tasks.EditOpts{DependsOn: &bad})
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid error, got %v", err)
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error should mention the missing ID, got: %v", err)
	}
}

// --- Dependents ---

// TestDependentsHappyPath checks that Dependents returns tasks depending on a given ID.
func TestDependentsHappyPath(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha")                                     // 1
	_, err := tasks.Create(boardDir, "beta", "", nil, []string{"1"}, "") // 2 depends on 1
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	deps, err := tasks.Dependents(boardDir, "1")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(deps) != 1 || deps[0].ID != "2" {
		t.Errorf("expected [2], got %v", deps)
	}
}

// TestDependentsNone checks that Dependents returns an empty non-nil slice when
// no tasks depend on the given ID.
func TestDependentsNone(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // 1, no dependents
	deps, err := tasks.Dependents(boardDir, "1")
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
	mustCreate(t, boardDir, "alpha")                                      // 1
	active := mustActive(t, boardDir, "beta")                             // 2 active
	_, err := tasks.Create(boardDir, "gamma", "", nil, []string{"1"}, "") // 3 draft
	if err != nil {
		t.Fatalf("create gamma: %v", err)
	}
	if _, err := tasks.SetState(boardDir, active.ID, "archive"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	// Add a dep from archived 2 to 1 via Edit.
	dep := []string{"1"}
	if _, err := tasks.Edit(boardDir, "2", tasks.EditOpts{DependsOn: &dep}); err != nil {
		t.Fatalf("edit 2 deps: %v", err)
	}
	deps, err := tasks.Dependents(boardDir, "1")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	// 3 (draft) and 2 (archive) both depend on 1.
	if len(deps) != 2 {
		t.Errorf("expected 2 dependents, got %d: %v", len(deps), deps)
	}
}

// --- End-to-end: Create with --depends-on ---

// TestCreateDependsOnE2E is an end-to-end test of the depends_on wiring in Create.
func TestCreateDependsOnE2E(t *testing.T) {
	boardDir := newBoard(t)

	// Pointing to nonexistent ID must fail.
	_, err := tasks.Create(boardDir, "orphan", "", nil, []string{"42"}, "")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid for missing dep, got %v", err)
	}

	// Create the dep first, then a task that references it — must succeed.
	mustCreate(t, boardDir, "foundation") // 1
	child, err := tasks.Create(boardDir, "child", "", nil, []string{"1"}, "")
	if err != nil {
		t.Fatalf("expected success with real dep, got %v", err)
	}
	if len(child.DependsOn) != 1 || child.DependsOn[0] != "1" {
		t.Errorf("depends_on not stored: %v", child.DependsOn)
	}
}

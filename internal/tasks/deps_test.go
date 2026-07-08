package tasks_test

import (
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/tasks"
)

// --- validateDeps (exercised through Create and Edit) ---

// TestValidateDepsHappyPath checks that Create succeeds when all deps exist.
func TestValidateDepsHappyPath(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // 1
	mustCreate(t, boardDir, "beta")  // 2
	_, _, err := tasks.Create(boardDir, "gamma", "", nil, []string{"1", "2"}, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestValidateDepsNotFound checks that Create returns Invalid when a dep is missing.
func TestValidateDepsNotFound(t *testing.T) {
	boardDir := newBoard(t)
	_, _, err := tasks.Create(boardDir, "gamma", "", nil, []string{"999"}, "")
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
	if _, _, err := tasks.Create(boardDir, "alpha", "", nil, nil, ""); err != nil {
		t.Fatalf("nil deps: %v", err)
	}
	if _, _, err := tasks.Create(boardDir, "beta", "", nil, []string{}, ""); err != nil {
		t.Fatalf("empty deps: %v", err)
	}
}

// TestValidateDepsEditNilSkipsValidation checks that Edit with nil DependsOn
// leaves the existing deps unchanged (nil means "leave unchanged").
func TestValidateDepsEditNilSkipsValidation(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "alpha") // 1
	body := "updated"
	edited, _, err := tasks.Edit(boardDir, "1", tasks.EditOpts{Body: &body})
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
	_, _, err := tasks.Edit(boardDir, "1", tasks.EditOpts{DependsOn: &bad})
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
	mustCreate(t, boardDir, "alpha")                                        // 1
	_, _, err := tasks.Create(boardDir, "beta", "", nil, []string{"1"}, "") // 2 depends on 1
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
	mustCreate(t, boardDir, "alpha")                                         // 1
	active := mustActive(t, boardDir, "beta")                                // 2 active
	_, _, err := tasks.Create(boardDir, "gamma", "", nil, []string{"1"}, "") // 3 draft
	if err != nil {
		t.Fatalf("create gamma: %v", err)
	}
	if _, err := tasks.SetState(boardDir, active.ID, "archive"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	// Add a dep from archived 2 to 1 via Edit.
	dep := []string{"1"}
	if _, _, err := tasks.Edit(boardDir, "2", tasks.EditOpts{DependsOn: &dep}); err != nil {
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
	_, _, err := tasks.Create(boardDir, "orphan", "", nil, []string{"42"}, "")
	if !isBoardErr(err, "invalid") {
		t.Fatalf("expected invalid for missing dep, got %v", err)
	}

	// Create the dep first, then a task that references it — must succeed.
	mustCreate(t, boardDir, "foundation") // 1
	child, _, err := tasks.Create(boardDir, "child", "", nil, []string{"1"}, "")
	if err != nil {
		t.Fatalf("expected success with real dep, got %v", err)
	}
	if len(child.DependsOn) != 1 || child.DependsOn[0] != "1" {
		t.Errorf("depends_on not stored: %v", child.DependsOn)
	}
}

// --- deps_enforcement levels ---

func setLevel(t *testing.T, boardDir string, level board.DepsEnforcement) {
	t.Helper()
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DepsEnforcement = level
	if _, err := board.WriteConfig(boardDir, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestHintAllowsForwardRefsWithWarning(t *testing.T) {
	boardDir := newBoard(t)
	setLevel(t, boardDir, board.DepsHint)
	task, warns, err := tasks.Create(boardDir, "early", "", nil, []string{"99"}, "")
	if err != nil {
		t.Fatalf("hint must allow a forward ref: %v", err)
	}
	if len(warns) == 0 || !strings.Contains(warns[0], "99") {
		t.Errorf("expected a forward-ref warning, got %v", warns)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "99" {
		t.Errorf("forward ref not stored: %v", task.DependsOn)
	}
}

func TestValidatedRefusesSelfAndCycle(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a") // 1
	mustCreate(t, boardDir, "b") // 2
	// Self-reference.
	self := []string{"1"}
	if _, _, err := tasks.Edit(boardDir, "1", tasks.EditOpts{DependsOn: &self}); !isBoardErr(err, "invalid") {
		t.Errorf("self-ref should be invalid at validated, got %v", err)
	}
	// Cycle: 2 → 1, then 1 → 2.
	dep := []string{"1"}
	if _, _, err := tasks.Edit(boardDir, "2", tasks.EditOpts{DependsOn: &dep}); err != nil {
		t.Fatal(err)
	}
	back := []string{"2"}
	if _, _, err := tasks.Edit(boardDir, "1", tasks.EditOpts{DependsOn: &back}); !isBoardErr(err, "invalid") {
		t.Errorf("cycle should be invalid at validated, got %v", err)
	}
}

func TestHintWarnsOnSelfAndCycle(t *testing.T) {
	boardDir := newBoard(t)
	setLevel(t, boardDir, board.DepsHint)
	mustCreate(t, boardDir, "a") // 1
	mustCreate(t, boardDir, "b") // 2
	dep := []string{"1"}
	if _, _, err := tasks.Edit(boardDir, "2", tasks.EditOpts{DependsOn: &dep}); err != nil {
		t.Fatal(err)
	}
	back := []string{"2"}
	_, warns, err := tasks.Edit(boardDir, "1", tasks.EditOpts{DependsOn: &back})
	if err != nil {
		t.Fatalf("hint must allow a cycle (warned): %v", err)
	}
	if len(warns) == 0 || !strings.Contains(warns[0], "cycle") {
		t.Errorf("expected a cycle warning, got %v", warns)
	}
}

func TestStatusMoveWithUnmetDeps(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "dep")                                                           // 1 (ready)
	if _, _, err := tasks.Create(boardDir, "work", "", nil, []string{"1"}, ""); err != nil { // 2
		t.Fatal(err)
	}

	// validated (default): done with unmet deps warns but succeeds.
	_, warns, err := tasks.SetStatus(boardDir, "2", "done")
	if err != nil {
		t.Fatalf("validated must allow done with unmet deps: %v", err)
	}
	if len(warns) == 0 || !strings.Contains(warns[0], "unmet") {
		t.Errorf("expected an unmet-deps warning, got %v", warns)
	}
	if _, _, err := tasks.SetStatus(boardDir, "2", "ready"); err != nil {
		t.Fatal(err)
	}

	// strict: refused with conflict…
	setLevel(t, boardDir, board.DepsStrict)
	if _, _, err := tasks.SetStatus(boardDir, "2", "in_progress"); !isBoardErr(err, "conflict") {
		t.Errorf("strict should refuse in_progress with unmet deps, got %v", err)
	}
	// …until the dep resolves.
	if _, _, err := tasks.SetStatus(boardDir, "1", "done"); err != nil {
		t.Fatal(err)
	}
	if _, warns, err := tasks.SetStatus(boardDir, "2", "done"); err != nil || len(warns) != 0 {
		t.Errorf("strict move after deps met: %v %v", err, warns)
	}
}

func TestDeleteHealsDependentsAtValidated(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "base")                                                           // 1
	if _, _, err := tasks.Create(boardDir, "child", "", nil, []string{"1"}, ""); err != nil { // 2
		t.Fatal(err)
	}
	warns, err := tasks.Delete(boardDir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) == 0 || !strings.Contains(warns[0], "removed") {
		t.Errorf("expected a healing note, got %v", warns)
	}
	child, err := tasks.Get(boardDir, "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(child.DependsOn) != 0 {
		t.Errorf("dependent not healed: %v", child.DependsOn)
	}
}

func TestDeleteLeavesDanglingAtHint(t *testing.T) {
	boardDir := newBoard(t)
	setLevel(t, boardDir, board.DepsHint)
	mustCreate(t, boardDir, "base")                                                           // 1
	if _, _, err := tasks.Create(boardDir, "child", "", nil, []string{"1"}, ""); err != nil { // 2
		t.Fatal(err)
	}
	warns, err := tasks.Delete(boardDir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) == 0 || !strings.Contains(warns[0], "dangle") {
		t.Errorf("expected a dangling warning, got %v", warns)
	}
	child, _ := tasks.Get(boardDir, "2")
	if len(child.DependsOn) != 1 || child.DependsOn[0] != "1" {
		t.Errorf("hint delete must leave refs untouched: %v", child.DependsOn)
	}
}

func TestDepsDeduped(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "a") // 1
	task, _, err := tasks.Create(boardDir, "b", "", nil, []string{"1", "1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(task.DependsOn) != 1 {
		t.Errorf("deps not deduped: %v", task.DependsOn)
	}
}

func TestUnmetDeps(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "open")     // 1 ready
	mustCreate(t, boardDir, "finished") // 2
	if _, _, err := tasks.SetStatus(boardDir, "2", "done"); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, boardDir, "gone") // 3
	if _, err := tasks.SetState(boardDir, "3", "archive"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tasks.Create(boardDir, "work", "", nil, []string{"1", "2", "3"}, ""); err != nil { // 4
		t.Fatal(err)
	}
	snap, err := tasks.Load(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	unmet := snap.UnmetDeps(snap.Get("4"))
	if len(unmet) != 1 || unmet[0] != "1" {
		t.Errorf("done and archived deps must count resolved: unmet = %v", unmet)
	}
}

func TestDoctorFixesDanglingDeps(t *testing.T) {
	boardDir := newBoard(t)
	setLevel(t, boardDir, board.DepsHint)
	if _, _, err := tasks.Create(boardDir, "early", "", nil, []string{"99"}, ""); err != nil {
		t.Fatal(err)
	}
	issues, err := tasks.Doctor(boardDir, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range issues {
		if i.Kind == "dangling-dep" {
			found = true
			if !i.Fixed {
				t.Errorf("dangling dep not fixed: %v", i)
			}
		}
	}
	if !found {
		t.Fatal("dangling-dep issue not reported")
	}
	task, _ := tasks.Get(boardDir, "1")
	if len(task.DependsOn) != 0 {
		t.Errorf("dangling ref not removed: %v", task.DependsOn)
	}
}

package tasks_test

import (
	"os"
	"path/filepath"
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
	if _, _, err := tasks.SetState(boardDir, active.ID, "archive"); err != nil {
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
	if err := board.SetConfigValue(boardDir, "deps_enforcement", string(level)); err != nil {
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
	if _, warns, err := tasks.SetStatus(boardDir, "2", "done"); err != nil ||
		strings.Contains(strings.Join(warns, " "), "unmet") {
		t.Errorf("strict move after deps met: %v %v", err, warns)
	}
}

func TestDeleteHealsDependentsAtValidated(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "base")                                                           // 1
	if _, _, err := tasks.Create(boardDir, "child", "", nil, []string{"1"}, ""); err != nil { // 2
		t.Fatal(err)
	}
	_, warns, err := tasks.Delete(boardDir, "1")
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
	_, warns, err := tasks.Delete(boardDir, "1")
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

// TestDeleteHealsBrokenDependent checks that a dependent whose file the
// snapshot excluded (unknown status) is still healed surgically: only the
// depends_on entry is removed, everything else — the bogus status included —
// stays byte-identical.
func TestDeleteHealsBrokenDependent(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "base") // 1
	writeRaw(t, boardDir, "tasks", "2-broken.md",
		"---\nid: \"2\"\ntitle: broken\nstatus: bogus\ndepends_on: [\"1\"]\n---\n\nkeep this body\n")

	_, warns, err := tasks.Delete(boardDir, "1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "2-broken.md") || !strings.Contains(joined, "removed it from depends_on") {
		t.Errorf("expected a broken-dependent healing warning, got %v", warns)
	}
	data, err := os.ReadFile(filepath.Join(boardDir, "tasks", "2-broken.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"1"`) {
		t.Errorf("deleted id still referenced:\n%s", data)
	}
	if !strings.Contains(string(data), "status: bogus") {
		t.Errorf("healing touched more than depends_on:\n%s", data)
	}
	if !strings.Contains(string(data), "keep this body") {
		t.Errorf("body lost:\n%s", data)
	}
}

// TestDeleteWarnsUncheckableBrokenFile checks that a file whose frontmatter
// cannot be read at all is left untouched, with a warning that it could not
// be checked.
func TestDeleteWarnsUncheckableBrokenFile(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "base")                                   // 1
	content := "---\nid: \"2\"\ntitle: broken\ndepends_on: [\"1\"]\n" // unterminated fence
	writeRaw(t, boardDir, "tasks", "2-broken.md", content)

	_, warns, err := tasks.Delete(boardDir, "1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "2-broken.md") || !strings.Contains(joined, "could not be checked") {
		t.Errorf("expected a could-not-be-checked warning, got %v", warns)
	}
	data, err := os.ReadFile(filepath.Join(boardDir, "tasks", "2-broken.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("unreadable file was modified:\n%s", data)
	}
}

// TestDeleteMakesOneCommit checks that deleting a task with several dependents
// records the removal and every heal in a single commit, leaving a clean tree.
func TestDeleteMakesOneCommit(t *testing.T) {
	boardDir := initRepo(t)
	if err := board.SetConfigValue(boardDir, "auto_commit", "true"); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, boardDir, "base") // 1
	for _, title := range []string{"child a", "child b", "child c"} {
		if _, _, err := tasks.Create(boardDir, title, "", nil, []string{"1"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	before := len(commitMessages(t, boardDir))

	if _, _, err := tasks.Delete(boardDir, "1"); err != nil {
		t.Fatal(err)
	}
	msgs := commitMessages(t, boardDir)
	if len(msgs)-before != 1 {
		t.Errorf("expected exactly one delete commit, got %d: %v", len(msgs)-before, msgs)
	}
	if msgs[0] != "north: delete 1" {
		t.Errorf("unexpected commit message %q", msgs[0])
	}
	// Task files only: init's scaffolding is never auto-committed.
	status, err := gitOut(t, boardDir, "status", "--porcelain", "--", "drafts", "tasks", "archive")
	if err != nil {
		t.Fatal(err)
	}
	if status != "" {
		t.Errorf("task files not committed after delete:\n%s", status)
	}
}

// TestDeleteHealsDuplicateIDDependent checks that a dependent excluded from the
// snapshot for a duplicate id — a well-formed file — heals through the normal
// path: updated_at is bumped and the warning talks about the duplicate id
// rather than calling the file broken.
func TestDeleteHealsDuplicateIDDependent(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "base") // 1
	mustCreate(t, boardDir, "dup")  // 2, drafts/2-dup.md — keeps the id
	writeRaw(t, boardDir, "tasks", "2-dup.md",
		"---\nid: \"2\"\ntitle: dup\nstatus: ready\nupdated_at: \"2020-01-01T00:00:00Z\"\n"+
			"depends_on: [\"1\"]\n---\n\nbody\n")

	_, warns, err := tasks.Delete(boardDir, "1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "2-dup.md has a duplicate id but depended on 1") {
		t.Errorf("expected a duplicate-id healing warning, got %v", warns)
	}
	if strings.Contains(joined, "2-dup.md is broken") {
		t.Errorf("a duplicate-id file must not be called broken: %v", warns)
	}
	data, err := os.ReadFile(filepath.Join(boardDir, "tasks", "2-dup.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"1"`) {
		t.Errorf("deleted id still referenced:\n%s", data)
	}
	if strings.Contains(string(data), "2020-01-01") {
		t.Errorf("duplicate-id heal must bump updated_at:\n%s", data)
	}
}

// TestDeleteHealsRightFileWhenNamesCollide checks that healing follows the
// warning's path, not its base name: two files can share a name across state
// folders, and only the one that actually carries the dependency is rewritten.
func TestDeleteHealsRightFileWhenNamesCollide(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "base") // 1
	// drafts/2-dup.md holds id 2 and no deps; the same-named active file is the
	// duplicate, and is the only one referencing the deleted task.
	original := "---\nid: \"2\"\ntitle: dup\nstatus: ready\n---\n\ndraft body\n"
	writeRaw(t, boardDir, "drafts", "2-dup.md", original)
	writeRaw(t, boardDir, "tasks", "2-dup.md",
		"---\nid: \"2\"\ntitle: dup\nstatus: ready\ndepends_on: [\"1\"]\n---\n\nactive body\n")

	if _, _, err := tasks.Delete(boardDir, "1"); err != nil {
		t.Fatal(err)
	}
	draft, err := os.ReadFile(filepath.Join(boardDir, "drafts", "2-dup.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(draft) != original {
		t.Errorf("the wrong file was rewritten:\n%s", draft)
	}
	active, err := os.ReadFile(filepath.Join(boardDir, "tasks", "2-dup.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(active), `"1"`) {
		t.Errorf("the dependent file was not healed:\n%s", active)
	}
}

// TestDeleteHealDoesNotClobberIDNamesake guards the heal paths against being
// routed through persist, whose target path is recomputed from id+title. Here
// the duplicate-id dependent also has filename drift, so that computed path is
// the *other* task's file: healing there overwrote one task and deleted the
// other. Heals must rewrite in place.
func TestDeleteHealDoesNotClobberIDNamesake(t *testing.T) {
	boardDir := newBoard(t)
	// Both files claim id "1" and title "a"; 2-a.md is the drifted duplicate
	// and the only one depending on the task being deleted.
	keeper := "---\nid: \"1\"\ntitle: a\nstatus: ready\n---\n\nkeeper body\n"
	writeRaw(t, boardDir, "tasks", "1-a.md", keeper)
	writeRaw(t, boardDir, "tasks", "2-a.md",
		"---\nid: \"1\"\ntitle: a\nstatus: ready\ndepends_on: [\"3\"]\n---\n\nduplicate body\n")
	writeRaw(t, boardDir, "tasks", "3-target.md",
		"---\nid: \"3\"\ntitle: target\nstatus: ready\n---\n")

	if _, _, err := tasks.Delete(boardDir, "3"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(boardDir, "tasks", "1-a.md"))
	if err != nil {
		t.Fatalf("the namesake task was destroyed: %v", err)
	}
	if string(got) != keeper {
		t.Errorf("the namesake task was overwritten:\n%s", got)
	}
	dup, err := os.ReadFile(filepath.Join(boardDir, "tasks", "2-a.md"))
	if err != nil {
		t.Fatalf("the healed file was moved away: %v", err)
	}
	if strings.Contains(string(dup), `"3"`) {
		t.Errorf("dependent not healed:\n%s", dup)
	}
	if !strings.Contains(string(dup), "duplicate body") {
		t.Errorf("healed file lost its body:\n%s", dup)
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
	if _, _, err := tasks.SetState(boardDir, "3", "archive"); err != nil {
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

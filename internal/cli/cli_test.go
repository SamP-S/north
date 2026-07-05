package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
)

// run executes the north command tree with args in the given working dir,
// returning combined stdout/stderr and the cobra error.
func run(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// runIn is like run but feeds stdin for interactive prompts.
func runIn(t *testing.T, dir, stdin string, args ...string) (string, error) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestCLIInitAndCreate(t *testing.T) {
	dir := t.TempDir()
	if out, err := run(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (%s)", err, out)
	}
	if _, err := board.LocateBoard(dir); err != nil {
		t.Fatalf("board not created: %v", err)
	}
	out, err := run(t, dir, "task", "create", "Add login", "--agent", "opus4.8")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(out, "Created 1 (draft): Add login") {
		t.Errorf("create output: %q", out)
	}
}

func TestCLIInitRefusesNested(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Re-init in the same dir and in a subdir must both refuse.
	if _, err := run(t, dir, "init"); err == nil || !strings.Contains(err.Error(), "already initialised") {
		t.Errorf("same-dir re-init: %v", err)
	}
	if _, err := run(t, sub, "init"); err == nil || !strings.Contains(err.Error(), "already initialised") {
		t.Errorf("nested init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "north")); !os.IsNotExist(err) {
		t.Error("nested board was created despite refusal")
	}
}

func TestCLIStateMoveBoard(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	// Status change on a draft is rejected.
	if _, err := run(t, dir, "task", "move", "1", "in_progress"); err == nil {
		t.Error("expected move on draft to fail")
	}
	if out, err := run(t, dir, "task", "state", "1", "active"); err != nil || !strings.Contains(out, "1 state → active") {
		t.Errorf("state: %q %v", out, err)
	}
	if out, err := run(t, dir, "task", "move", "1", "in_progress"); err != nil || !strings.Contains(out, "1 → in_progress") {
		t.Errorf("move: %q %v", out, err)
	}
	// Freeform: jump straight to failed, then back to ready.
	if _, err := run(t, dir, "task", "move", "1", "failed"); err != nil {
		t.Errorf("freeform in_progress→failed: %v", err)
	}
	if _, err := run(t, dir, "task", "move", "1", "ready"); err != nil {
		t.Errorf("freeform failed→ready: %v", err)
	}
	out, err := run(t, dir, "board")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "in_progress") || !strings.Contains(out, "total") {
		t.Errorf("board output: %q", out)
	}
}

func TestCLIStateFreeform(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	// draft → archive directly (skip the board), then archive → active directly.
	if out, err := run(t, dir, "task", "state", "1", "archive"); err != nil {
		t.Fatalf("draft→archive: %v (%s)", out, err)
	}
	if out, err := run(t, dir, "task", "state", "1", "active"); err != nil {
		t.Fatalf("archive→active: %v (%s)", out, err)
	}
}

func TestCLIBoardJSONAndPlain(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "state", "1", "active")
	run(t, dir, "task", "move", "1", "in_progress")

	out, err := run(t, dir, "board", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"active"`) || !strings.Contains(out, `"in_progress": 1`) ||
		!strings.Contains(out, `"drafts"`) || !strings.Contains(out, `"warnings"`) {
		t.Errorf("board --json output: %q", out)
	}

	out, err = run(t, dir, "board", "--plain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "in_progress\t1") || !strings.Contains(out, "drafts\t0") || !strings.Contains(out, "archive\t0") {
		t.Errorf("board --plain output: %q", out)
	}
}

func TestCLIListJSON(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	// Created tasks are drafts; default list (active) is empty.
	out, err := run(t, dir, "task", "list", "--state", "draft", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"tasks"`) || !strings.Contains(out, `"id": "1"`) ||
		!strings.Contains(out, `"state": "draft"`) || !strings.Contains(out, `"warnings"`) {
		t.Errorf("json output: %q", out)
	}
}

func TestCLIListSearchAndLabel(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "fix the login page", "--labels", "auth,web")
	run(t, dir, "task", "create", "write docs", "--labels", "docs")

	out, _ := run(t, dir, "task", "list", "--state", "draft", "--search", "LOGIN", "--plain")
	if !strings.Contains(out, "fix the login page") || strings.Contains(out, "write docs") {
		t.Errorf("--search: %q", out)
	}
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--label", "docs", "--plain")
	if !strings.Contains(out, "write docs") || strings.Contains(out, "login") {
		t.Errorf("--label: %q", out)
	}
}

func TestCLIListWarnsOnBadFile(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "good")
	bad := filepath.Join(dir, "north", "drafts", "9-bad.md")
	if err := os.WriteFile(bad, []byte("no frontmatter at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, dir, "task", "list", "--state", "all", "--plain")
	if err != nil {
		t.Fatalf("one bad file must not break list: %v", err)
	}
	if !strings.Contains(out, "good") {
		t.Errorf("good task missing: %q", out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "9-bad.md") {
		t.Errorf("expected warning naming the bad file: %q", out)
	}
}

func TestCLIDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	if out, err := run(t, dir, "task", "delete", "1", "--yes"); err != nil || !strings.Contains(out, "Deleted 1") {
		t.Errorf("delete: %q %v", out, err)
	}
	if out, _ := run(t, dir, "task", "list", "--state", "all", "--plain"); strings.TrimSpace(out) != "" {
		t.Errorf("expected empty list, got %q", out)
	}
}

func TestCLIDeleteOutputFlags(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := run(t, dir, "task", "delete", "1", "--yes", "--plain")
	if err != nil || !strings.Contains(out, "id:         1") {
		t.Errorf("delete --plain: %q %v", out, err)
	}

	run(t, dir, "task", "create", "y")
	out, err = run(t, dir, "task", "delete", "1", "--yes", "--json")
	if err != nil || !strings.Contains(out, `"id": "1"`) {
		t.Errorf("delete --json: %q %v", out, err)
	}
}

func TestCLIDeleteMachineModeRequiresYes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	// --json/--plain without -y must refuse instead of prompting.
	if _, err := run(t, dir, "task", "delete", "1", "--json"); err == nil {
		t.Error("delete --json without -y should refuse")
	}
	if _, err := run(t, dir, "task", "delete", "1", "--plain"); err == nil {
		t.Error("delete --plain without -y should refuse")
	}
	// Task untouched.
	if _, err := run(t, dir, "task", "view", "1"); err != nil {
		t.Errorf("task should survive refused deletes: %v", err)
	}
}

func TestCLISkillShow(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, dir, "skill", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "name: north") || !strings.Contains(out, "north-skill-version:") {
		t.Errorf("skill show output: %q", out)
	}
}

func TestCLISkillInstall(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	out, err := run(t, dir, "skill", "install")
	if err != nil {
		t.Fatalf("skill install: %v (%s)", err, out)
	}
	for _, p := range []string{
		filepath.Join(dir, ".claude", "skills", "north", "SKILL.md"),
		filepath.Join(dir, ".opencode", "skills", "north", "SKILL.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s: %v", p, err)
		}
	}
}

func TestCLISkillCheck(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// Before install: missing → non-zero exit.
	if _, err := run(t, dir, "skill", "check"); err == nil {
		t.Error("skill check should fail before install")
	}
	run(t, dir, "skill", "install")
	out, err := run(t, dir, "skill", "check")
	if err != nil {
		t.Fatalf("skill check after install: %v (%s)", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("skill check output: %q", out)
	}
}

func TestCLINoBoardErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := run(t, dir, "task", "list")
	if err == nil {
		t.Error("expected error when no board present")
	}
}

func TestCLIView(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "Add login")
	out, err := run(t, dir, "task", "view", "1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"state": "draft"`) || !strings.Contains(out, `"status": "ready"`) {
		t.Errorf("view json: %q", out)
	}
	if pl, _ := run(t, dir, "task", "view", "1", "--plain"); !strings.Contains(pl, "state:") {
		t.Errorf("view plain: %q", pl)
	}
}

func TestCLIEditBody(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	if _, err := run(t, dir, "task", "edit", "1", "--body", "updated body"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, _ := run(t, dir, "task", "view", "1", "--plain")
	if !strings.Contains(out, "updated body") {
		t.Errorf("body not applied: %q", out)
	}
}

func TestCLIEditAppendAndBodyFile(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x", "--body", "original")
	if _, err := run(t, dir, "task", "edit", "1", "--append-body", "progress note"); err != nil {
		t.Fatalf("append-body: %v", err)
	}
	out, _ := run(t, dir, "task", "view", "1", "--plain")
	if !strings.Contains(out, "original") || !strings.Contains(out, "progress note") {
		t.Errorf("append lost content: %q", out)
	}
	// --append-body is exclusive with --body.
	if _, err := run(t, dir, "task", "edit", "1", "--append-body", "a", "--body", "b"); err == nil {
		t.Error("append-body + body should be rejected")
	}
	// edit --body-file replaces the body from a file.
	bf := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bf, []byte("from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "task", "edit", "1", "--body-file", bf); err != nil {
		t.Fatalf("edit --body-file: %v", err)
	}
	out, _ = run(t, dir, "task", "view", "1", "--plain")
	if !strings.Contains(out, "from file") || strings.Contains(out, "original") {
		t.Errorf("body-file not applied: %q", out)
	}
}

func TestCLICleanupMessages(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if out, _ := run(t, dir, "cleanup"); !strings.Contains(out, "Nothing to clean up") {
		t.Errorf("empty cleanup: %q", out)
	}
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "state", "1", "active")
	run(t, dir, "task", "move", "1", "done")
	if out, _ := run(t, dir, "cleanup"); !strings.Contains(out, "Archived 1") {
		t.Errorf("cleanup: %q", out)
	}
}

func TestCLIDeleteDeclined(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := runIn(t, dir, "n\n", "task", "delete", "1")
	if err == nil {
		t.Error("declining should exit non-zero")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("expected Aborted: %q", out)
	}
	// Task still exists.
	if _, err := run(t, dir, "task", "view", "1"); err != nil {
		t.Errorf("task should survive a declined delete: %v", err)
	}
}

func TestCLIDeleteConfirmed(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := runIn(t, dir, "y\n", "task", "delete", "1")
	if err != nil || !strings.Contains(out, "Deleted 1") {
		t.Errorf("confirmed delete: %q %v", out, err)
	}
}

func TestCLIDeleteWarnsDependents(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "base")
	run(t, dir, "task", "create", "dep", "--depends-on", "1")
	out, err := run(t, dir, "task", "delete", "1", "--yes")
	if err != nil {
		t.Fatalf("delete: %v (%s)", err, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "2") {
		t.Errorf("expected warning about 2 in output: %q", out)
	}
	// 2 should still exist (warn-but-allow)
	if _, err := run(t, dir, "task", "view", "2"); err != nil {
		t.Errorf("2 should still exist after deleting 1: %v", err)
	}
}

func TestCLIDeleteWarnsJSON(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "base")
	run(t, dir, "task", "create", "dep", "--depends-on", "1")
	out, err := run(t, dir, "task", "delete", "1", "--yes", "--json")
	if err != nil {
		t.Fatalf("delete --json: %v (%s)", err, out)
	}
	if !strings.Contains(out, `"warnings"`) {
		t.Errorf("expected warnings key in JSON: %q", out)
	}
	if !strings.Contains(out, `"2"`) {
		t.Errorf("expected 2 in warnings: %q", out)
	}
}

func TestCLICreateRejectsInvalidDep(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	_, err := run(t, dir, "task", "create", "x", "--depends-on", "99")
	if err == nil {
		t.Error("expected error when depends-on references non-existent task")
	}
}

func TestCLIConfigGetSet(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if out, err := run(t, dir, "config", "get", "auto_commit"); err != nil || strings.TrimSpace(out) != "false" {
		t.Errorf("config get default: %q %v", out, err)
	}
	if _, err := run(t, dir, "config", "set", "auto_commit", "true"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if out, _ := run(t, dir, "config", "get", "auto_commit"); strings.TrimSpace(out) != "true" {
		t.Errorf("config get after set: %q", out)
	}
	if out, _ := run(t, dir, "config", "list"); !strings.Contains(out, "auto_commit: true") {
		t.Errorf("config list: %q", out)
	}
	if _, err := run(t, dir, "config", "set", "auto_commit", "banana"); err == nil {
		t.Error("bad bool should be rejected")
	}
	if _, err := run(t, dir, "config", "get", "nonsense"); err == nil {
		t.Error("unknown key should be rejected")
	}
}

func TestCLIDoctor(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "fine")
	if out, err := run(t, dir, "doctor"); err != nil || !strings.Contains(out, "healthy") {
		t.Errorf("clean doctor: %q %v", out, err)
	}
	// Inject a duplicate id; doctor must exit non-zero, then --fix repairs it.
	dup := filepath.Join(dir, "north", "drafts", "1-dup.md")
	if err := os.WriteFile(dup, []byte("---\nid: \"1\"\ntitle: dup\nstatus: ready\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "doctor"); err == nil {
		t.Error("doctor with issues should exit non-zero")
	}
	if out, err := run(t, dir, "doctor", "--fix"); err != nil {
		t.Errorf("doctor --fix should exit zero when everything was fixed: %q %v", out, err)
	}
	if out, err := run(t, dir, "doctor"); err != nil || !strings.Contains(out, "healthy") {
		t.Errorf("doctor after fix: %q %v", out, err)
	}
}

func TestCLIJSONError(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// Executing a failing command with --json through Execute()'s path.
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	root := newRootCmd()
	root.SetArgs([]string{"task", "view", "99", "--json"})
	cmd, err := root.ExecuteC()
	if err == nil {
		t.Fatal("expected error")
	}
	if !jsonRequested(cmd) {
		t.Error("jsonRequested should detect --json on the failing command")
	}
}

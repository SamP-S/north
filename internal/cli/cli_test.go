package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
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
	out, err := run(t, dir, "task", "create", "Add login", "--assignee", "opus4.8")
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
	// Status change on a draft is allowed (freeform on both axes) — it just
	// isn't visible on the board until the task is active.
	if out, err := run(t, dir, "task", "move", "1", "in_progress"); err != nil || !strings.Contains(out, "1 → in_progress") {
		t.Errorf("move on draft: %q %v", out, err)
	}
	if _, err := run(t, dir, "task", "move", "1", "ready"); err != nil {
		t.Errorf("move draft back to ready: %v", err)
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

func TestCLILabelFlagAliases(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// create accepts the singular alias for --labels.
	out, err := run(t, dir, "task", "create", "aliased", "--label", "x", "--json")
	if err != nil {
		t.Fatalf("create --label: %v (%s)", err, out)
	}
	if !strings.Contains(out, `"labels": [`) || !strings.Contains(out, `"x"`) {
		t.Errorf("create --label should set labels: %q", out)
	}
	// edit accepts the singular alias too.
	if out, err := run(t, dir, "task", "edit", "1", "--label", "y"); err != nil {
		t.Fatalf("edit --label: %v (%s)", err, out)
	}
	// list accepts the plural alias for --label.
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--labels", "y", "--plain")
	if !strings.Contains(out, "aliased") {
		t.Errorf("list --labels should filter: %q", out)
	}
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--labels", "zzz", "--plain")
	if strings.Contains(out, "aliased") {
		t.Errorf("list --labels zzz should exclude: %q", out)
	}
	// Help still documents only the canonical names.
	out, _ = run(t, dir, "task", "create", "--help")
	if !strings.Contains(out, "--labels") || strings.Contains(out, "--label ") {
		t.Errorf("create help should show only --labels: %q", out)
	}
	out, _ = run(t, dir, "task", "list", "--help")
	if !strings.Contains(out, "--label ") || strings.Contains(out, "--labels") {
		t.Errorf("list help should show only --label: %q", out)
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
	if err != nil || !strings.HasPrefix(out, "1\tdraft\tready") {
		t.Errorf("delete --plain should print a list row: %q %v", out, err)
	}

	// Deleted ids are never reused: the next create gets 2.
	run(t, dir, "task", "create", "y")
	out, err = run(t, dir, "task", "delete", "2", "--yes", "--json")
	if err != nil || !strings.Contains(out, `"id": "2"`) {
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
	// Before install: nothing anywhere → non-zero exit.
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

func TestCLISkillInstallTarget(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	out, err := run(t, dir, "skill", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("selective install: %v (%s)", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "north", "SKILL.md")); err != nil {
		t.Error("claude skill should be installed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode")); !os.IsNotExist(err) {
		t.Error("opencode dir should not be created for --target claude")
	}

	// A selective install must pass check: missing is a warning, not an error.
	out, err = run(t, dir, "skill", "check")
	if err != nil {
		t.Fatalf("check after selective install: %v (%s)", err, out)
	}
	if !strings.Contains(out, "up to date") || !strings.Contains(out, "not installed") {
		t.Errorf("check output: %q", out)
	}

	// Unknown target is rejected as invalid (exit 2).
	_, err = run(t, dir, "skill", "install", "--target", "cursor")
	if errorCode(t, err) != "invalid" || !strings.Contains(err.Error(), "unknown skill target") {
		t.Errorf("unknown target should be invalid: %v", err)
	}
}

func TestCLISkillInstallIOErrorStaysInternal(t *testing.T) {
	dir := t.TempDir()
	// A plain file where the skill dir should go makes MkdirAll fail; that is
	// an I/O failure, not a usage mistake, so it must not become invalid.
	if err := os.WriteFile(filepath.Join(dir, ".claude"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, dir, "skill", "install", "--target", "claude")
	if err == nil {
		t.Fatal("install into a blocked dir should fail")
	}
	if _, ok := nerrors.As(err); ok {
		t.Errorf("I/O failure should stay internal (exit 1), got typed error %v", err)
	}
}

func TestCLISkillCheckOutputModes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "skill", "install", "--target", "claude")

	// claude ok + opencode missing → exit 0, both rows in the payload.
	out, err := run(t, dir, "skill", "check", "--json")
	if err != nil {
		t.Fatalf("check --json: %v (%s)", err, out)
	}
	var payload struct {
		Targets []struct {
			Agent     string `json:"agent"`
			Path      string `json:"path"`
			Installed string `json:"installed"`
			Binary    string `json:"binary"`
			Status    string `json:"status"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("check --json is not valid JSON: %v (%s)", err, out)
	}
	if len(payload.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(payload.Targets))
	}
	statuses := map[string]string{}
	for _, tg := range payload.Targets {
		statuses[tg.Agent] = tg.Status
		if tg.Path == "" || tg.Binary == "" {
			t.Errorf("target %q missing path/binary: %+v", tg.Agent, tg)
		}
	}
	if statuses["Claude Code"] != "ok" || statuses["opencode"] != "missing" {
		t.Errorf("statuses = %v", statuses)
	}

	// Plain: one tab-separated row per target.
	pl, err := run(t, dir, "skill", "check", "--plain")
	if err != nil {
		t.Fatalf("check --plain: %v", err)
	}
	if !strings.Contains(pl, "Claude Code\tok\t") || !strings.Contains(pl, "opencode\tmissing\t\t") {
		t.Errorf("check plain: %q", pl)
	}

	// Stale stamp → outdated status and conflict exit, in JSON mode too.
	stale := filepath.Join(dir, ".claude", "skills", "north", "SKILL.md")
	if err := os.WriteFile(stale, []byte("<!-- north-skill-version: 0.0.0-old -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, dir, "skill", "check", "--json")
	if errorCode(t, err) != "conflict" {
		t.Errorf("outdated should be conflict, got %v", err)
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("outdated check --json: %v (%s)", err, out)
	}
	found := false
	for _, tg := range payload.Targets {
		if tg.Agent == "Claude Code" {
			found = true
			if tg.Status != "outdated" || tg.Installed != "0.0.0-old" {
				t.Errorf("outdated target: %+v", tg)
			}
		}
	}
	if !found {
		t.Error("Claude Code target missing from payload")
	}
}

func TestCLIVersionOutputModes(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, dir, "version")
	if err != nil || !strings.HasPrefix(out, "north ") {
		t.Errorf("version: %q %v", out, err)
	}
	pl, err := run(t, dir, "version", "--plain")
	if err != nil || strings.Contains(pl, "north ") || strings.TrimSpace(pl) == "" {
		t.Errorf("version --plain: %q %v", pl, err)
	}
	js, err := run(t, dir, "version", "--json")
	if err != nil {
		t.Fatalf("version --json: %v", err)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(js), &payload); err != nil || payload.Version != strings.TrimSpace(pl) {
		t.Errorf("version --json: %q (%v), plain %q", js, err, pl)
	}
}

func TestCLITuiRejectsArgs(t *testing.T) {
	dir := t.TempDir()
	// Args validation runs before RunE, so the TUI never launches.
	_, err := run(t, dir, "tui", "extra-arg")
	if errorCode(t, err) != "invalid" {
		t.Errorf("tui with positional args should be invalid usage, got %v", err)
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

// TestCLIViewJSONEnvelope verifies view --json wraps the task in the same
// {"task": …, "warnings": []} envelope every mutation payload uses.
func TestCLIViewJSONEnvelope(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "Add login")
	out, err := run(t, dir, "task", "view", "1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Task     map[string]any `json:"task"`
		Warnings []string       `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if payload.Task["id"] != "1" || payload.Task["title"] != "Add login" {
		t.Errorf("task envelope: %v", payload.Task)
	}
	if payload.Warnings == nil {
		t.Error("warnings should be [] not null")
	}
}

// TestCLIInactiveNoteInJSONWarnings verifies the draft/archive advisory from
// move/state rides the payload warnings instead of a bare stderr note.
func TestCLIInactiveNoteInJSONWarnings(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	out, err := run(t, dir, "task", "move", "1", "in_progress", "--json")
	if err != nil {
		t.Fatalf("move: %v (%s)", err, out)
	}
	var payload struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(payload.Warnings) == 0 || !strings.Contains(payload.Warnings[0], "status shows on the board once active") {
		t.Errorf("move on a draft should carry the inactive note: %v", payload.Warnings)
	}
	// state to archive carries it too (resulting state not active)…
	out, err = run(t, dir, "task", "state", "1", "archive", "--json")
	if err != nil {
		t.Fatalf("state: %v (%s)", err, out)
	}
	payload.Warnings = nil
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(payload.Warnings) == 0 || !strings.Contains(payload.Warnings[0], "once active") {
		t.Errorf("state to archive should carry the inactive note: %v", payload.Warnings)
	}
	// …and activation does not.
	out, err = run(t, dir, "task", "state", "1", "active", "--json")
	if err != nil {
		t.Fatalf("state: %v (%s)", err, out)
	}
	payload.Warnings = nil
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(payload.Warnings) != 0 {
		t.Errorf("activation should carry no inactive note: %v", payload.Warnings)
	}
}

// TestCLICleanupJSONWarnings verifies cleanup --json surfaces snapshot
// warnings (e.g. a malformed task file) instead of a hardcoded empty list.
func TestCLICleanupJSONWarnings(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "good")
	run(t, dir, "task", "state", "1", "active")
	run(t, dir, "task", "move", "1", "done")
	bad := filepath.Join(dir, "north", "tasks", "9-bad.md")
	if err := os.WriteFile(bad, []byte("no frontmatter at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, dir, "cleanup", "--json")
	if err != nil {
		t.Fatalf("cleanup: %v (%s)", err, out)
	}
	var payload struct {
		Tasks    []map[string]any `json:"tasks"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(payload.Tasks) != 1 {
		t.Fatalf("expected 1 archived task, got %v", payload.Tasks)
	}
	if len(payload.Warnings) == 0 || !strings.Contains(payload.Warnings[0], "9-bad.md") {
		t.Errorf("expected snapshot warning naming the bad file: %v", payload.Warnings)
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
	if nerrors.ExitCode(err) != 1 {
		t.Errorf("declined delete should exit 1 (abort): %v", err)
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("expected Aborted: %q", out)
	}
	// Task still exists.
	if _, err := run(t, dir, "task", "view", "1"); err != nil {
		t.Errorf("task should survive a declined delete: %v", err)
	}
}

func TestCLIDeleteNonInteractiveRequiresYes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")

	// A redirected stdin (/dev/null is a char device but not a terminal) must
	// hit the -y guard, not the interactive prompt.
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()

	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetIn(devnull)
	root.SetArgs([]string{"task", "delete", "1"})
	err = root.Execute()
	if err == nil {
		t.Fatalf("non-interactive delete without -y should fail: %q", buf.String())
	}
	if be, ok := nerrors.As(err); !ok || be.Code() != "invalid" {
		t.Errorf("expected invalid error, got: %v", err)
	}
	if strings.Contains(buf.String(), "[y/N]") {
		t.Errorf("must not prompt on non-TTY stdin: %q", buf.String())
	}
	if _, err := run(t, dir, "task", "view", "1"); err != nil {
		t.Errorf("task should survive: %v", err)
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
	// Default level (validated) heals the dependent and says so.
	if !strings.Contains(out, "removed 1 from depends_on of 2") {
		t.Errorf("expected healing note in warnings: %q", out)
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

func TestCLIConfigOutputModes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// list --json: typed values under a "config" wrapper.
	out, err := run(t, dir, "config", "list", "--json")
	if err != nil {
		t.Fatalf("config list --json: %v (%s)", err, out)
	}
	var listPayload struct {
		Config struct {
			Version         int    `json:"version"`
			AutoCommit      bool   `json:"auto_commit"`
			DepsEnforcement string `json:"deps_enforcement"`
			MaxWIP          int    `json:"max_wip"`
			LastID          int    `json:"last_id"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(out), &listPayload); err != nil {
		t.Fatalf("config list --json unmarshal: %v (%s)", err, out)
	}
	c := listPayload.Config
	if c.Version != 1 || c.AutoCommit || c.DepsEnforcement != "validated" || c.MaxWIP != 0 || c.LastID != 0 {
		t.Errorf("config list --json payload: %+v", c)
	}
	if strings.Contains(out, `"version": "1"`) {
		t.Errorf("list --json must carry typed values, not strings: %s", out)
	}
	// list --plain: tab-separated key\tvalue lines.
	if out, _ := run(t, dir, "config", "list", "--plain"); !strings.Contains(out, "max_wip\t0\n") {
		t.Errorf("config list --plain: %q", out)
	}
	// get --json: {"key": …, "value": …} with a typed value.
	if out, _ = run(t, dir, "config", "get", "max_wip", "--json"); !strings.Contains(out, `"key": "max_wip"`) || !strings.Contains(out, `"value": 0`) {
		t.Errorf("config get --json: %q", out)
	}
	// get --plain: the bare value, same as human.
	if out, _ = run(t, dir, "config", "get", "max_wip", "--plain"); strings.TrimSpace(out) != "0" {
		t.Errorf("config get --plain: %q", out)
	}
	// set --json echoes the new typed value.
	if out, err = run(t, dir, "config", "set", "max_wip", "3", "--json"); err != nil ||
		!strings.Contains(out, `"key": "max_wip"`) || !strings.Contains(out, `"value": 3`) {
		t.Errorf("config set --json: %q %v", out, err)
	}
	// set --plain: key\tvalue.
	if out, err = run(t, dir, "config", "set", "auto_commit", "true", "--plain"); err != nil || strings.TrimSpace(out) != "auto_commit\ttrue" {
		t.Errorf("config set --plain: %q %v", out, err)
	}
}

func TestCLIConfigSetPreservesFile(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// Bump last_id via task creation, then set an unrelated key: the set must
	// keep the scaffold's comments and never rewind the id high-water mark.
	run(t, dir, "task", "create", "x")
	if _, err := run(t, dir, "config", "set", "max_wip", "2"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "north", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#") {
		t.Errorf("config comments should survive config set: %q", data)
	}
	if !strings.Contains(string(data), "last_id: 1") {
		t.Errorf("config set must not rewind last_id: %q", data)
	}
	if out, _ := run(t, dir, "config", "get", "last_id"); strings.TrimSpace(out) != "1" {
		t.Errorf("last_id after config set: %q", out)
	}
}

func TestCLIDoctor(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "fine")
	if out, err := run(t, dir, "doctor"); err != nil || !strings.Contains(out, "healthy") {
		t.Errorf("clean doctor: %q %v", out, err)
	}
	// Inject a duplicate id; doctor reports it (exit 0 — findings are output,
	// not failure), then --fix repairs it.
	dup := filepath.Join(dir, "north", "drafts", "1-dup.md")
	if err := os.WriteFile(dup, []byte("---\nid: \"1\"\ntitle: dup\nstatus: ready\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, dir, "doctor"); err != nil || !strings.Contains(out, "duplicate-id") {
		t.Errorf("doctor should report the duplicate and exit 0: %q %v", out, err)
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

func errorCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	be, ok := nerrors.As(err)
	if !ok {
		return "internal"
	}
	return be.Code()
}

func TestCLIExitCodeContract(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// not_found → 3.
	_, err := run(t, dir, "task", "view", "99")
	if got := nerrors.ExitCode(err); got != 3 {
		t.Errorf("not_found exit = %d, want 3 (%v)", got, err)
	}
	// invalid (bad status) → 2.
	run(t, dir, "task", "create", "x")
	_, err = run(t, dir, "task", "move", "1", "bogus")
	if got := nerrors.ExitCode(err); got != 2 {
		t.Errorf("invalid exit = %d, want 2 (%v)", got, err)
	}
	// usage (wrong arg count, unknown flag) → 2.
	_, err = run(t, dir, "task", "move", "1")
	if got := nerrors.ExitCode(err); got != 2 {
		t.Errorf("arg-count exit = %d, want 2 (%v)", got, err)
	}
	_, err = run(t, dir, "task", "list", "--bogus")
	if got := nerrors.ExitCode(err); got != 2 {
		t.Errorf("unknown-flag exit = %d, want 2 (%v)", got, err)
	}
	// conflict (re-init) → 4.
	_, err = run(t, dir, "init")
	if got := nerrors.ExitCode(err); got != 4 {
		t.Errorf("conflict exit = %d, want 4 (%v)", got, err)
	}
	// Unknown subcommand → 2. Execute maps it by sniffing cobra's error text;
	// this pins the wording so a cobra reword breaks loudly here, not by
	// silently flipping the exit code to 1 in the field.
	_, err = run(t, dir, "bogus-subcommand")
	if err == nil {
		t.Fatal("unknown subcommand should error")
	}
	if _, ok := nerrors.As(err); ok || !strings.HasPrefix(err.Error(), "unknown command") {
		t.Errorf("unknown subcommand error = %q, want plain cobra error with %q prefix", err, "unknown command")
	}
}

func TestCLIBatchMove(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "a")
	run(t, dir, "task", "create", "b")
	run(t, dir, "task", "state", "1,2", "active")
	out, err := run(t, dir, "task", "move", "1,2,1", "done")
	if err != nil {
		t.Fatalf("batch move: %v (%s)", err, out)
	}
	if !strings.Contains(out, "1 → done") || !strings.Contains(out, "2 → done") {
		t.Errorf("per-id report missing: %q", out)
	}
	if strings.Count(out, "1 → done") != 1 {
		t.Errorf("duplicate id not deduplicated: %q", out)
	}
}

func TestCLIBatchContinuesOnError(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "a")
	out, err := run(t, dir, "task", "move", "1,99", "done")
	if err == nil {
		t.Fatal("partial failure must exit non-zero")
	}
	if code := errorCode(t, err); code != "not_found" {
		t.Errorf("shared failure code = %q, want not_found", code)
	}
	if !strings.Contains(out, "1 → done") {
		t.Errorf("surviving id should still be processed: %q", out)
	}
	if !strings.Contains(out, "error [not_found]") {
		t.Errorf("per-id error report missing: %q", out)
	}
	// The good task really moved.
	if v, _ := run(t, dir, "task", "view", "1", "--plain"); !strings.Contains(v, "status:     done") {
		t.Errorf("task 1 not moved: %q", v)
	}
}

func TestCLIBatchJSON(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "a")
	out, err := run(t, dir, "task", "move", "1,99", "done", "--json")
	if err == nil {
		t.Fatal("expected non-zero on partial failure")
	}
	if !strings.Contains(out, `"tasks"`) || !strings.Contains(out, `"errors"`) ||
		!strings.Contains(out, `"not_found"`) {
		t.Errorf("batch json payload: %q", out)
	}
}

func TestCLIBatchDelete(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "a")
	run(t, dir, "task", "create", "b")
	// Batch delete without -y refuses.
	if _, err := run(t, dir, "task", "delete", "1,2"); err == nil {
		t.Error("batch delete without -y should refuse")
	}
	out, err := run(t, dir, "task", "delete", "1,2", "-y")
	if err != nil {
		t.Fatalf("batch delete: %v (%s)", err, out)
	}
	if !strings.Contains(out, "Deleted 1") || !strings.Contains(out, "Deleted 2") {
		t.Errorf("per-id report: %q", out)
	}
	if l, _ := run(t, dir, "task", "list", "--state", "all", "--plain"); strings.TrimSpace(l) != "" {
		t.Errorf("tasks remain: %q", l)
	}
}

func TestCLIListAssigneeFilter(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "mine", "--assignee", "claude")
	run(t, dir, "task", "create", "yours", "--assignee", "john")
	run(t, dir, "task", "create", "nobody")
	out, _ := run(t, dir, "task", "list", "--state", "draft", "--assignee", "claude", "--plain")
	if !strings.Contains(out, "mine") || strings.Contains(out, "yours") || strings.Contains(out, "nobody") {
		t.Errorf("--assignee claude: %q", out)
	}
	// Empty value matches unassigned tasks.
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--assignee", "", "--plain")
	if !strings.Contains(out, "nobody") || strings.Contains(out, "mine") {
		t.Errorf("--assignee '': %q", out)
	}
}

func TestCLIPlainListColumns(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x", "--assignee", "claude", "--labels", "auth,web")
	out, _ := run(t, dir, "task", "list", "--state", "draft", "--plain")
	if !strings.Contains(out, "1\tdraft\tready\tclaude\tauth,web\tx") {
		t.Errorf("plain columns: %q", out)
	}
}

func TestCLICreateFillsTemplate(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	// Bodyless create fills from north/task-template.md.
	run(t, dir, "task", "create", "scaffolded")
	out, _ := run(t, dir, "task", "view", "1", "--plain")
	if !strings.Contains(out, "## Summary") || !strings.Contains(out, "## Acceptance Criteria") {
		t.Errorf("template not applied: %q", out)
	}
	// An explicit body wins.
	run(t, dir, "task", "create", "explicit", "--body", "my body")
	out, _ = run(t, dir, "task", "view", "2", "--plain")
	if strings.Contains(out, "## Summary") || !strings.Contains(out, "my body") {
		t.Errorf("--body should override template: %q", out)
	}
	// A deleted template means a blank body, not the embedded default.
	if err := os.Remove(filepath.Join(dir, "north", "task-template.md")); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "task", "create", "blank")
	out, _ = run(t, dir, "task", "view", "3", "--plain")
	if strings.Contains(out, "## Summary") {
		t.Errorf("missing template must mean blank body: %q", out)
	}
}

func TestCLIInitEpilogueAndModes(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, dir, "init")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Optional next steps") ||
		!strings.Contains(out, "north skill install") ||
		!strings.Contains(out, "auto_commit true") {
		t.Errorf("epilogue missing: %q", out)
	}
	dir2 := t.TempDir()
	out, err = run(t, dir2, "init", "--plain")
	if err != nil || strings.Contains(out, "Optional next steps") {
		t.Errorf("--plain must suppress the epilogue: %q %v", out, err)
	}
	dir3 := t.TempDir()
	out, err = run(t, dir3, "init", "--json")
	if err != nil || !strings.Contains(out, `"board"`) || strings.Contains(out, "Optional") {
		t.Errorf("--json init: %q %v", out, err)
	}
}

func TestCLIConfigVersionReadOnly(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if out, err := run(t, dir, "config", "get", "version"); err != nil || strings.TrimSpace(out) != "1" {
		t.Errorf("config get version: %q %v", out, err)
	}
	if out, _ := run(t, dir, "config", "list"); !strings.Contains(out, "version: 1") {
		t.Errorf("config list: %q", out)
	}
	_, err := run(t, dir, "config", "set", "version", "2")
	if err == nil || errorCode(t, err) != "invalid" {
		t.Errorf("config set version should refuse as invalid: %v", err)
	}
}

func TestCLINewerBoardRefused(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, "north", "config.yml"),
		[]byte("version: 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, dir, "task", "list")
	if err == nil || errorCode(t, err) != "conflict" ||
		!strings.Contains(err.Error(), "newer north") {
		t.Errorf("newer board should refuse with conflict: %v", err)
	}
	// init must not scaffold a nested board under a newer one.
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, sub, "init"); err == nil {
		t.Error("init under a newer board should refuse")
	}
	if _, err := os.Stat(filepath.Join(sub, "north")); !os.IsNotExist(err) {
		t.Error("nested board was created under a newer board")
	}
}

func TestCLIListSort(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "banana")
	run(t, dir, "task", "create", "apple")

	// Default: id descending (newest first).
	out, _ := run(t, dir, "task", "list", "--state", "draft", "--plain")
	if !strings.Contains(out, "2\tdraft") || strings.Index(out, "apple") > strings.Index(out, "banana") {
		t.Errorf("default sort should be newest first: %q", out)
	}
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--sort", "title", "--plain")
	if strings.Index(out, "apple") > strings.Index(out, "banana") {
		t.Errorf("--sort title should be A→Z: %q", out)
	}
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--sort", "title", "--reverse", "--plain")
	if strings.Index(out, "banana") > strings.Index(out, "apple") {
		t.Errorf("--sort title --reverse should be Z→A: %q", out)
	}
	if _, err := run(t, dir, "task", "list", "--sort", "priority"); err == nil {
		t.Error("unknown sort key should be rejected")
	}
}

func TestCLIDepsFilter(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "dep")                          // 1
	run(t, dir, "task", "create", "waiting", "--depends-on", "1") // 2
	run(t, dir, "task", "create", "free")                         // 3

	out, err := run(t, dir, "task", "list", "--state", "draft", "--deps", "unmet", "--plain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "waiting") || strings.Contains(out, "free") {
		t.Errorf("--deps unmet: %q", out)
	}
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--deps", "met", "--plain")
	if strings.Contains(out, "waiting") || !strings.Contains(out, "free") {
		t.Errorf("--deps met: %q", out)
	}
	// Resolving the dep moves 2 across.
	run(t, dir, "task", "move", "1", "done")
	out, _ = run(t, dir, "task", "list", "--state", "draft", "--deps", "unmet", "--plain")
	if strings.Contains(out, "waiting") {
		t.Errorf("resolved dep should clear unmet: %q", out)
	}
	if _, err := run(t, dir, "task", "list", "--deps", "banana"); err == nil {
		t.Error("bad --deps value should be rejected")
	}
}

func TestCLIDepsEnforcementConfig(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if out, err := run(t, dir, "config", "get", "deps_enforcement"); err != nil || strings.TrimSpace(out) != "validated" {
		t.Errorf("default level: %q %v", out, err)
	}
	if _, err := run(t, dir, "config", "set", "deps_enforcement", "strict"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, dir, "config", "get", "deps_enforcement"); strings.TrimSpace(out) != "strict" {
		t.Errorf("after set: %q", out)
	}
	if _, err := run(t, dir, "config", "set", "deps_enforcement", "lenient"); err == nil {
		t.Error("unknown level should be rejected")
	}

	// Strict refuses finishing with unmet deps, exit 4.
	run(t, dir, "task", "create", "dep")                       // 1
	run(t, dir, "task", "create", "work", "--depends-on", "1") // 2
	_, err := run(t, dir, "task", "move", "2", "done")
	if got := nerrors.ExitCode(err); got != 4 {
		t.Errorf("strict refusal exit = %d, want 4 (%v)", got, err)
	}
	// Warnings reach the JSON payload at validated.
	run(t, dir, "config", "set", "deps_enforcement", "validated")
	out, err := run(t, dir, "task", "move", "2", "done", "--json")
	if err != nil {
		t.Fatalf("validated move: %v (%s)", err, out)
	}
	if !strings.Contains(out, `"warnings"`) || !strings.Contains(out, "unmet") {
		t.Errorf("expected unmet warning in JSON: %q", out)
	}
}

func TestCLIMutationPlainPrintsRow(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	out, err := run(t, dir, "task", "create", "Row shape", "--labels", "a,b", "--plain")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	row := strings.TrimSuffix(out, "\n")
	if strings.Contains(row, "\n") {
		t.Fatalf("plain mutation should print one row: %q", out)
	}
	cols := strings.Split(row, "\t")
	if len(cols) != 6 || cols[0] != "1" || cols[1] != "draft" || cols[5] != "Row shape" {
		t.Fatalf("unexpected columns: %q", row)
	}
	for _, args := range [][]string{
		{"task", "state", "1", "active", "--plain"},
		{"task", "move", "1", "done", "--plain"},
		{"task", "edit", "1", "--assignee", "sam", "--plain"},
		{"task", "delete", "1", "-y", "--plain"},
	} {
		out, err := run(t, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got := strings.Count(strings.TrimSuffix(out, "\n"), "\n"); got != 0 {
			t.Errorf("%v should print one row, got %q", args, out)
		}
		if len(strings.Split(strings.TrimSuffix(out, "\n"), "\t")) != 6 {
			t.Errorf("%v: not a 6-column row: %q", args, out)
		}
	}
}

func TestCLIListEmptyPlainPrintsNothing(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	out, err := run(t, dir, "task", "list", "--plain")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if out != "" {
		t.Errorf("empty plain list should print nothing, got %q", out)
	}
}

func TestCLIListLimit(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	for _, title := range []string{"a", "b", "c"} {
		run(t, dir, "task", "create", title)
	}
	out, err := run(t, dir, "task", "list", "--state", "draft", "--limit", "2", "--plain")
	if err != nil {
		t.Fatalf("list --limit: %v", err)
	}
	if rows := strings.Split(strings.TrimSpace(out), "\n"); len(rows) != 2 {
		t.Errorf("want 2 rows, got %d: %q", len(rows), out)
	}
	if _, err := run(t, dir, "task", "list", "--limit", "-1"); err == nil {
		t.Error("negative --limit should be invalid")
	}
}

func TestCLIConfigLastID(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "create", "y")
	if out, err := run(t, dir, "config", "get", "last_id"); err != nil || strings.TrimSpace(out) != "2" {
		t.Fatalf("last_id after two creates: %q %v", out, err)
	}
	// Read-only.
	if _, err := run(t, dir, "config", "set", "last_id", "99"); err == nil {
		t.Error("config set last_id should be refused")
	}
	// Deleting the newest task must not free its id.
	run(t, dir, "task", "delete", "2", "-y")
	out, err := run(t, dir, "task", "create", "z", "--json")
	if err != nil || !strings.Contains(out, `"id": "3"`) {
		t.Fatalf("id 2 must not be reused: %q %v", out, err)
	}
	// The allocation rewrite preserves the scaffold's comments.
	data, err := os.ReadFile(filepath.Join(dir, "north", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#") {
		t.Errorf("config comments should survive allocation: %q", data)
	}
}

func TestCLICleanupRejectsNegativeOlderThan(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "state", "1", "active")
	run(t, dir, "task", "move", "1", "done")
	_, err := run(t, dir, "cleanup", "--older-than", "-1")
	if got := nerrors.ExitCode(err); got != 2 {
		t.Errorf("negative --older-than exit = %d, want 2 (%v)", got, err)
	}
	if err == nil || !strings.Contains(err.Error(), "--older-than must not be negative") {
		t.Errorf("expected the negative --older-than message, got %v", err)
	}
	// Nothing was archived by the refused run.
	out, _ := run(t, dir, "task", "view", "1", "--json")
	if !strings.Contains(out, `"state": "active"`) {
		t.Errorf("refused cleanup must not archive: %q", out)
	}
}

// TestCLIMutationJSONWrapped pins the mutation commands' JSON contract:
// {"task": {…}, "warnings": […]} — the same wrapper next/take use, with
// warnings always an array, never null.
func TestCLIMutationJSONWrapped(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	for _, args := range [][]string{
		{"task", "create", "Wrapped", "--json"},
		{"task", "state", "1", "active", "--json"},
		{"task", "move", "1", "done", "--json"},
		{"task", "edit", "1", "--assignee", "sam", "--json"},
		{"task", "delete", "1", "-y", "--json"},
	} {
		out, err := run(t, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v (%s)", args, err, out)
		}
		var payload struct {
			Task     map[string]any `json:"task"`
			Warnings []string       `json:"warnings"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("%v: bad json %q: %v", args, out, err)
		}
		if payload.Task == nil || payload.Task["id"] != "1" {
			t.Errorf("%v: task not wrapped under \"task\": %q", args, out)
		}
		if payload.Warnings == nil {
			t.Errorf("%v: warnings must be an array, never null: %q", args, out)
		}
	}
}

func TestCLICleanupJSONDryRunKey(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "task", "create", "x")
	run(t, dir, "task", "state", "1", "active")
	run(t, dir, "task", "move", "1", "done")
	out, err := run(t, dir, "cleanup", "--dry-run", "--json")
	if err != nil || !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("dry-run payload should say so: %q %v", out, err)
	}
	out, err = run(t, dir, "cleanup", "--json")
	if err != nil || !strings.Contains(out, `"dry_run": false`) {
		t.Fatalf("real run payload should say so: %q %v", out, err)
	}
}

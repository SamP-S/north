package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nerrors "github.com/SamP-S/north/internal/errors"
)

// pickBoard scaffolds a board with one active ready task ("1").
func pickBoard(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := run(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "task", "create", "First task"); err != nil {
		t.Fatalf("create: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "task", "state", "1", "active"); err != nil {
		t.Fatalf("state: %v (%s)", err, out)
	}
	return dir
}

// pickJSON parses a next/take --json payload's "task" value (nil when null).
func pickJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var payload struct {
		Task     map[string]any `json:"task"`
		Warnings []string       `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	return payload.Task
}

func TestCLINextAndTake(t *testing.T) {
	dir := pickBoard(t)

	out, err := run(t, dir, "next", "--json")
	if err != nil {
		t.Fatalf("next: %v (%s)", err, out)
	}
	task := pickJSON(t, out)
	if task == nil || task["id"] != "1" || task["status"] != "ready" {
		t.Fatalf("unexpected next payload: %v", task)
	}

	out, err = run(t, dir, "take", "--assignee", "agent-a", "--json")
	if err != nil {
		t.Fatalf("take: %v (%s)", err, out)
	}
	task = pickJSON(t, out)
	if task == nil || task["id"] != "1" || task["status"] != "in_progress" || task["assignee"] != "agent-a" {
		t.Fatalf("unexpected take payload: %v", task)
	}

	// Board drained → both report the empty contract with exit 0.
	for _, cmd := range []string{"next", "take"} {
		args := []string{cmd, "--json"}
		if cmd == "take" {
			args = append(args, "--assignee", "agent-b")
		}
		out, err = run(t, dir, args...)
		if err != nil {
			t.Fatalf("%s on empty board: %v (%s)", cmd, err, out)
		}
		if pickJSON(t, out) != nil {
			t.Fatalf("%s: expected null task, got %s", cmd, out)
		}
	}
	if out, err := run(t, dir, "next"); err != nil || !strings.Contains(out, "No workable task.") {
		t.Fatalf("human empty next: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "next", "--plain"); err != nil || strings.TrimSpace(out) != "" {
		t.Fatalf("plain empty next should print nothing: %v (%q)", err, out)
	}
}

func TestCLINextTakePlainSingleRow(t *testing.T) {
	dir := pickBoard(t)

	out, err := run(t, dir, "next", "--plain")
	if err != nil {
		t.Fatalf("next --plain: %v (%s)", err, out)
	}
	row := strings.TrimSuffix(out, "\n")
	if strings.Contains(row, "\n") {
		t.Fatalf("plain next should print one list row: %q", out)
	}
	cols := strings.Split(row, "\t")
	if len(cols) != 6 || cols[0] != "1" || cols[1] != "active" || cols[2] != "ready" || cols[5] != "First task" {
		t.Fatalf("unexpected plain next columns: %q", row)
	}

	out, err = run(t, dir, "take", "--assignee", "agent-a", "--plain")
	if err != nil {
		t.Fatalf("take --plain: %v (%s)", err, out)
	}
	cols = strings.Split(strings.TrimSuffix(out, "\n"), "\t")
	if len(cols) != 6 || cols[0] != "1" || cols[2] != "in_progress" || cols[3] != "agent-a" {
		t.Fatalf("unexpected plain take columns: %q", out)
	}
}

func TestCLITakeSpecificID(t *testing.T) {
	dir := pickBoard(t)
	if out, err := run(t, dir, "task", "create", "Second task"); err != nil {
		t.Fatalf("create: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "task", "state", "2", "active"); err != nil {
		t.Fatalf("state: %v (%s)", err, out)
	}

	out, err := run(t, dir, "take", "2", "--assignee", "agent-a", "--json")
	if err != nil {
		t.Fatalf("take 2: %v (%s)", err, out)
	}
	if task := pickJSON(t, out); task == nil || task["id"] != "2" || task["status"] != "in_progress" {
		t.Fatalf("unexpected payload: %v", task)
	}
	// Taken → conflict; --label with an id → invalid.
	_, err = run(t, dir, "take", "2", "--assignee", "agent-b")
	if be, ok := nerrors.As(err); !ok || be.Code() != "conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
	_, err = run(t, dir, "take", "1", "--assignee", "agent-b", "--label", "x")
	if be, ok := nerrors.As(err); !ok || be.Code() != "invalid" {
		t.Fatalf("expected invalid for --label with id, got %v", err)
	}
}

func TestCLINextTakeLabelAlias(t *testing.T) {
	dir := pickBoard(t)
	if out, err := run(t, dir, "task", "edit", "1", "--labels", "x"); err != nil {
		t.Fatalf("edit: %v (%s)", err, out)
	}
	// next accepts the plural alias for --label.
	out, err := run(t, dir, "next", "--labels", "x", "--json")
	if err != nil {
		t.Fatalf("next --labels: %v (%s)", err, out)
	}
	if task := pickJSON(t, out); task == nil || task["id"] != "1" {
		t.Fatalf("next --labels x should pick task 1: %v", task)
	}
	out, err = run(t, dir, "next", "--labels", "other", "--json")
	if err != nil {
		t.Fatalf("next --labels other: %v (%s)", err, out)
	}
	if task := pickJSON(t, out); task != nil {
		t.Fatalf("next --labels other should pick nothing: %v", task)
	}
	// take accepts the plural alias too.
	out, err = run(t, dir, "take", "--assignee", "agent-a", "--labels", "x", "--json")
	if err != nil {
		t.Fatalf("take --labels: %v (%s)", err, out)
	}
	if task := pickJSON(t, out); task == nil || task["id"] != "1" || task["status"] != "in_progress" {
		t.Fatalf("take --labels x should claim task 1: %v", task)
	}
}

func TestCLINextLimit(t *testing.T) {
	dir := pickBoard(t)
	if out, err := run(t, dir, "task", "create", "Second task"); err != nil {
		t.Fatalf("create: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "task", "state", "2", "active"); err != nil {
		t.Fatalf("state: %v (%s)", err, out)
	}

	out, err := run(t, dir, "next", "-l", "3", "--json")
	if err != nil {
		t.Fatalf("next -l 3: %v (%s)", err, out)
	}
	var payload struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(payload.Tasks) != 2 || payload.Tasks[0]["id"] != "1" || payload.Tasks[1]["id"] != "2" {
		t.Fatalf("unexpected tasks: %v", payload.Tasks)
	}
	// Plain renders list rows.
	if out, err := run(t, dir, "next", "-l", "2", "--plain"); err != nil || len(strings.Split(strings.TrimSpace(out), "\n")) != 2 {
		t.Fatalf("plain rows: %v (%q)", err, out)
	}
	// Limit 0 means all workable tasks, rendered as a list.
	out, err = run(t, dir, "next", "-l", "0", "--json")
	if err != nil {
		t.Fatalf("next -l 0: %v (%s)", err, out)
	}
	payload.Tasks = nil
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(payload.Tasks) != 2 {
		t.Fatalf("-l 0 should return all workable, got %v", payload.Tasks)
	}
	// Only negative limits are invalid.
	_, err = run(t, dir, "next", "-l", "-1")
	if be, ok := nerrors.As(err); !ok || be.Code() != "invalid" {
		t.Fatalf("expected invalid for -l -1, got %v", err)
	}
}

// TestCLINextLimitPlainEmpty verifies an empty pick under -l N --plain prints
// nothing at all — not even a blank line.
func TestCLINextLimitPlainEmpty(t *testing.T) {
	dir := t.TempDir()
	if out, err := run(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (%s)", err, out)
	}
	out, err := run(t, dir, "next", "-l", "2", "--plain")
	if err != nil {
		t.Fatalf("next -l 2 --plain: %v (%s)", err, out)
	}
	if out != "" {
		t.Fatalf("empty plain pick should print nothing, got %q", out)
	}
}

func TestCLICleanupDryRun(t *testing.T) {
	dir := pickBoard(t)
	if out, err := run(t, dir, "task", "move", "1", "done"); err != nil {
		t.Fatalf("move: %v (%s)", err, out)
	}
	out, err := run(t, dir, "cleanup", "--dry-run")
	if err != nil || !strings.Contains(out, "Would archive 1 done task(s): 1") {
		t.Fatalf("dry run: %v (%q)", err, out)
	}
	// Nothing moved.
	out, err = run(t, dir, "task", "view", "1", "--json")
	if err != nil || !strings.Contains(out, `"state": "active"`) {
		t.Fatalf("dry run mutated: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "cleanup"); err != nil || !strings.Contains(out, "Archived 1 done task(s): 1") {
		t.Fatalf("real cleanup: %v (%q)", err, out)
	}
}

func TestCLIMoveReadyAssignedWarns(t *testing.T) {
	dir := pickBoard(t)
	if out, err := run(t, dir, "take", "--assignee", "agent-a"); err != nil {
		t.Fatalf("take: %v (%s)", err, out)
	}
	out, err := run(t, dir, "task", "move", "1", "ready")
	if err != nil || !strings.Contains(out, "still assigned to \"agent-a\"") {
		t.Fatalf("expected still-assigned warning: %v (%q)", err, out)
	}
}

func TestCLITakeAssigneeFromEnv(t *testing.T) {
	dir := pickBoard(t)
	t.Setenv("NORTH_AGENT", "env-agent")
	out, err := run(t, dir, "take", "--json")
	if err != nil {
		t.Fatalf("take: %v (%s)", err, out)
	}
	if task := pickJSON(t, out); task == nil || task["assignee"] != "env-agent" {
		t.Fatalf("expected env assignee, got %v", task)
	}
}

func TestCLITakeNoAssigneeIsInvalid(t *testing.T) {
	dir := pickBoard(t)
	t.Setenv("NORTH_AGENT", "")
	_, err := run(t, dir, "take")
	if be, ok := nerrors.As(err); !ok || be.Code() != "invalid" {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestCLIConfigMaxWIP(t *testing.T) {
	dir := pickBoard(t)
	if out, err := run(t, dir, "config", "get", "max_wip"); err != nil || strings.TrimSpace(out) != "0" {
		t.Fatalf("default max_wip: %v (%q)", err, out)
	}
	if out, err := run(t, dir, "config", "set", "max_wip", "1"); err != nil {
		t.Fatalf("set: %v (%s)", err, out)
	}
	if _, err := run(t, dir, "config", "set", "max_wip", "-1"); err == nil {
		t.Fatal("negative max_wip accepted")
	}

	// Cap of 1: second take for the same assignee is a conflict.
	if out, err := run(t, dir, "take", "--assignee", "agent-a"); err != nil {
		t.Fatalf("take: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "task", "create", "Second task"); err != nil {
		t.Fatalf("create: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "task", "state", "2", "active"); err != nil {
		t.Fatalf("state: %v (%s)", err, out)
	}
	_, err := run(t, dir, "take", "--assignee", "agent-a")
	if be, ok := nerrors.As(err); !ok || be.Code() != "conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestCLIInitScaffoldsGitignore(t *testing.T) {
	dir := t.TempDir()
	if out, err := run(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (%s)", err, out)
	}
	gi := filepath.Join(dir, "north", ".gitignore")
	data, err := os.ReadFile(gi)
	if err != nil {
		t.Fatalf("gitignore not scaffolded: %v", err)
	}
	if !strings.Contains(string(data), ".lock") || !strings.Contains(string(data), "*.tmp") {
		t.Fatalf("unexpected gitignore content: %q", data)
	}

	// doctor: missing → reported; --fix → restored.
	if err := os.Remove(gi); err != nil {
		t.Fatal(err)
	}
	// Findings are output, not failure: a completed scan exits 0.
	if out, err := run(t, dir, "doctor"); err != nil || !strings.Contains(out, "gitignore") {
		t.Fatalf("doctor should flag missing gitignore and exit 0: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "doctor", "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v (%s)", err, out)
	}
	if _, err := os.Stat(gi); err != nil {
		t.Fatalf("gitignore not restored: %v", err)
	}
}

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
	if out, err := run(t, dir, "doctor"); err == nil || !strings.Contains(out, "gitignore") {
		t.Fatalf("doctor should flag missing gitignore: %v (%s)", err, out)
	}
	if out, err := run(t, dir, "doctor", "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v (%s)", err, out)
	}
	if _, err := os.Stat(gi); err != nil {
		t.Fatalf("gitignore not restored: %v", err)
	}
}

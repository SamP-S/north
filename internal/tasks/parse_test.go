package tasks_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// writeRaw drops a hand-written task file into a state folder, bypassing the
// normal Create path so the parser can be exercised against bad input.
func writeRaw(t *testing.T, boardDir, stateFolder, filename, content string) {
	t.Helper()
	p := filepath.Join(boardDir, stateFolder, filename)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsMalformedFiles(t *testing.T) {
	cases := []struct {
		name, file, content string
	}{
		{"unterminated frontmatter", "task-1-a.md", "---\nid: task-1\ntitle: a\n"},
		{"missing id", "task-2-a.md", "---\ntitle: a\nstatus: ready\n---\n"},
		{"missing title", "task-3-a.md", "---\nid: task-3\nstatus: ready\n---\n"},
		{"unknown status", "task-4-a.md", "---\nid: task-4\ntitle: a\nstatus: bogus\n---\n"},
		{"broken yaml", "task-5-a.md", "---\nid: [unclosed\n---\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			boardDir := newBoard(t)
			writeRaw(t, boardDir, "tasks", c.file, c.content)
			// Listing loads every file; a bad one must surface as Invalid.
			if _, err := tasks.List(boardDir, nil, ""); !isBoardErr(err, "invalid") {
				t.Fatalf("expected invalid, got %v", err)
			}
		})
	}
}

func TestLoadAcceptsMinimalValidFile(t *testing.T) {
	boardDir := newBoard(t)
	writeRaw(t, boardDir, "tasks", "task-7-hand.md",
		"---\nid: task-7\ntitle: hand written\nstatus: in_progress\n---\n\nbody text\n")
	task, err := tasks.Get(boardDir, "task-7")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != models.StateActive || task.Status != models.InProgress {
		t.Errorf("unexpected: state=%s status=%s", task.State, task.Status)
	}
	if task.Body != "body text" {
		t.Errorf("body: %q", task.Body)
	}
}

func TestRoundTripFidelity(t *testing.T) {
	boardDir := newBoard(t)
	// Pre-create the dep so validateDeps is satisfied.
	mustCreate(t, boardDir, "dep") // task-1
	// Unicode title and a body containing a literal "---" line.
	title := "Café — déjà vu"
	body := "first line\n\n---\na horizontal rule inside the body\n---\n\nlast line"
	created, err := tasks.Create(boardDir, title, "ollama:llama3", []string{"x", "y"}, []string{"task-1"}, body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tasks.Get(boardDir, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != title {
		t.Errorf("title: %q != %q", got.Title, title)
	}
	if got.Body != body {
		t.Errorf("body round-trip failed:\n got %q\nwant %q", got.Body, body)
	}
	if got.Agent != "ollama:llama3" || len(got.Labels) != 2 || len(got.DependsOn) != 1 {
		t.Errorf("fields lost: %+v", got)
	}
	// Timestamps persist at RFC3339 (second) precision.
	if got.CreatedAt == nil || got.CreatedAt.Format(time.RFC3339) != created.CreatedAt.Format(time.RFC3339) {
		t.Errorf("created_at not preserved: got %v want %v", got.CreatedAt, created.CreatedAt)
	}
}

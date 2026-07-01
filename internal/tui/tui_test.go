package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func TestParseEditorResult(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTitle  string
		wantBody   string
		wantAgent  string
		wantLabels []string
	}{
		{
			name:      "title only",
			input:     "# My Task\n",
			wantTitle: "My Task",
		},
		{
			name:      "title and body",
			input:     "# Fix login\n\nSome body text.\nSecond line.",
			wantTitle: "Fix login",
			wantBody:  "Some body text.\nSecond line.",
		},
		{
			name:       "title labels body",
			input:      "# Add auth\nlabels: backend, api\n\nImplement OAuth.",
			wantTitle:  "Add auth",
			wantBody:   "Implement OAuth.",
			wantLabels: []string{"backend", "api"},
		},
		{
			name:       "agent field",
			input:      "# Task\nagent: claude\nlabels: ops\n\nbody",
			wantTitle:  "Task",
			wantAgent:  "claude",
			wantBody:   "body",
			wantLabels: []string{"ops"},
		},
		{
			name:       "labels with spaces",
			input:      "# Task\nlabels:  foo ,  bar ,baz\n\nbody",
			wantTitle:  "Task",
			wantBody:   "body",
			wantLabels: []string{"foo", "bar", "baz"},
		},
		{
			name:      "no body",
			input:     "# Title\n",
			wantTitle: "Title",
			wantBody:  "",
		},
		{
			name:      "empty input",
			input:     "",
			wantTitle: "",
		},
		{
			name:      "trailing newlines stripped from body",
			input:     "# T\n\nbody\n\n",
			wantTitle: "T",
			wantBody:  "body",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, body, agent, labels := ParseEditorResult(tc.input)
			if title != tc.wantTitle {
				t.Errorf("title: got %q, want %q", title, tc.wantTitle)
			}
			if body != tc.wantBody {
				t.Errorf("body: got %q, want %q", body, tc.wantBody)
			}
			if agent != tc.wantAgent {
				t.Errorf("agent: got %q, want %q", agent, tc.wantAgent)
			}
			if len(labels) != len(tc.wantLabels) {
				t.Errorf("labels length: got %d, want %d (%v)", len(labels), len(tc.wantLabels), labels)
			} else {
				for i, l := range labels {
					if l != tc.wantLabels[i] {
						t.Errorf("labels[%d]: got %q, want %q", i, l, tc.wantLabels[i])
					}
				}
			}
		})
	}
}

func TestSortDescByID(t *testing.T) {
	make := func(id string) *models.Task { return &models.Task{ID: id} }
	tasks := []*models.Task{make("task-1"), make("task-10"), make("task-3"), make("task-2")}
	sortDescByID(tasks)
	want := []string{"task-10", "task-3", "task-2", "task-1"}
	for i, w := range want {
		if tasks[i].ID != w {
			t.Errorf("pos %d: got %q, want %q", i, tasks[i].ID, w)
		}
	}
}

func TestCreateTemplate(t *testing.T) {
	tmpl := createTemplate()
	if tmpl == "" {
		t.Fatal("createTemplate returned empty string")
	}
	title, _, _, _ := ParseEditorResult(tmpl)
	if title == "" {
		t.Error("template should parse to a non-empty title placeholder")
	}
}

// TestBoardDeleteWarningModal verifies that pressing 'd' when a dependent task
// exists produces a pendingText that names both the target and its dependent.
func TestBoardDeleteWarningModal(t *testing.T) {
	// Set up a real board in a temp directory.
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init board: %v", err)
	}

	// Create task-1 and make it active.
	t1, err := tasks.Create(dir, "First task", "", nil, nil, "")
	if err != nil {
		t.Fatalf("create task-1: %v", err)
	}
	t1, err = tasks.Promote(dir, t1.ID)
	if err != nil {
		t.Fatalf("promote task-1: %v", err)
	}

	// Create task-2, make it active, then set it to depend on task-1.
	t2, err := tasks.Create(dir, "Second task", "", nil, nil, "")
	if err != nil {
		t.Fatalf("create task-2: %v", err)
	}
	t2, err = tasks.Promote(dir, t2.ID)
	if err != nil {
		t.Fatalf("promote task-2: %v", err)
	}
	deps := []string{t1.ID}
	_, err = tasks.Edit(dir, t2.ID, nil, nil, nil, &deps, nil)
	if err != nil {
		t.Fatalf("edit task-2 depends_on: %v", err)
	}

	// Build a boardModel with task-1 selected in the first column.
	m := boardModel{
		boardDir: dir,
		columns: []boardColumn{
			{status: string(models.Ready), tasks: []*models.Task{t1}, cursor: 0},
		},
		colIdx: 0,
		width:  80,
		height: 24,
	}

	// Send the 'd' key.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	pt := updated.pendingText
	if !strings.Contains(pt, t1.ID) {
		t.Errorf("pendingText %q does not contain %q", pt, t1.ID)
	}
	if !strings.Contains(pt, t2.ID) {
		t.Errorf("pendingText %q does not contain %q (dependent)", pt, t2.ID)
	}
}

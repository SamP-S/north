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
		name          string
		input         string
		wantTitle     string
		wantBody      string
		wantAgent     string
		wantLabels    []string
		wantDependsOn []string
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
			name:          "depends_on field",
			input:         "# Task\ndepends_on: task-1, task-2\n\nbody",
			wantTitle:     "Task",
			wantBody:      "body",
			wantDependsOn: []string{"task-1", "task-2"},
		},
		{
			name:          "depends_on with spaces and blank entries",
			input:         "# Task\ndepends_on:  task-1 , , task-3 \n\nbody",
			wantTitle:     "Task",
			wantBody:      "body",
			wantDependsOn: []string{"task-1", "task-3"},
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
			title, body, agent, labels, dependsOn := ParseEditorResult(tc.input)
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
			if len(dependsOn) != len(tc.wantDependsOn) {
				t.Errorf("depends_on length: got %d, want %d (%v)", len(dependsOn), len(tc.wantDependsOn), dependsOn)
			} else {
				for i, d := range dependsOn {
					if d != tc.wantDependsOn[i] {
						t.Errorf("depends_on[%d]: got %q, want %q", i, d, tc.wantDependsOn[i])
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
	title, _, _, _, _ := ParseEditorResult(tmpl)
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

// TestListDeleteWarningModal verifies that the list view's delete confirm
// carries the same dependents warning as the board view's — the two views
// must behave identically for the same key.
func TestListDeleteWarningModal(t *testing.T) {
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init board: %v", err)
	}

	t1, err := tasks.Create(dir, "First task", "", nil, nil, "")
	if err != nil {
		t.Fatalf("create task-1: %v", err)
	}
	t2, err := tasks.Create(dir, "Second task", "", nil, []string{t1.ID}, "")
	if err != nil {
		t.Fatalf("create task-2: %v", err)
	}
	_ = t2

	m := listModel{
		boardDir:   dir,
		filtered:   []*models.Task{t1},
		activePane: paneList,
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if updated.confirm != confirmDelete {
		t.Fatalf("expected confirmDelete, got %v", updated.confirm)
	}
	if !strings.Contains(updated.confirmText, t1.ID) {
		t.Errorf("confirmText %q does not contain %q", updated.confirmText, t1.ID)
	}
	if !strings.Contains(updated.confirmText, t2.ID) {
		t.Errorf("confirmText %q does not contain %q (dependent)", updated.confirmText, t2.ID)
	}
}

// TestListMoveOpensStatusPicker verifies that 'm' opens the same status-picker
// modal in the list view as in the board view.
func TestListMoveOpensStatusPicker(t *testing.T) {
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init board: %v", err)
	}
	active, err := tasks.Create(dir, "Active task", "", nil, nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	active, err = tasks.Promote(dir, active.ID)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}

	m := listModel{
		boardDir:   dir,
		filtered:   []*models.Task{active},
		activePane: paneList,
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if updated.modal != modalStatusPicker {
		t.Fatalf("expected modalStatusPicker, got %v", updated.modal)
	}
}

// TestPromoteOrDemoteIgnoresArchived verifies that Promote (the 'p' key) is a
// no-op on archived tasks — restoring is Archive's ('a') job, so the two keys
// no longer overlap.
func TestPromoteOrDemoteIgnoresArchived(t *testing.T) {
	t1 := &models.Task{ID: "task-1", State: models.StateArchive}
	if cmd := promoteOrDemote("unused", t1); cmd != nil {
		t.Error("expected promoteOrDemote to be a no-op for an archived task")
	}
}

// isQuit reports whether cmd resolves to tea.QuitMsg.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestQuitIsNoOpOnOpenBoardModal verifies that pressing 'q' while the board's
// status-picker modal is open neither quits the app nor closes the modal —
// esc is the only cancel key. esc still closes it.
func TestQuitIsNoOpOnOpenBoardModal(t *testing.T) {
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init board: %v", err)
	}
	active, err := tasks.Create(dir, "Active task", "", nil, nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if active, err = tasks.Promote(dir, active.ID); err != nil {
		t.Fatalf("promote: %v", err)
	}

	m := NewModel(dir)
	m.width, m.height = 80, 24
	m.board.columns = []boardColumn{
		{status: string(models.Ready), tasks: []*models.Task{active}, cursor: 0},
	}
	m.board.modal = modalStatusPicker

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	um := updated.(Model)

	if um.board.modal != modalStatusPicker {
		t.Errorf("expected modal to stay open after q, got %v", um.board.modal)
	}
	if isQuit(cmd) {
		t.Error("q should not quit the app while a modal is open")
	}

	// esc still cancels.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).board.modal != modalNone {
		t.Error("expected esc to close the modal")
	}
}

// TestQuitIsNoOpOnOpenListConfirm verifies the same for the list view's
// delete/archive confirm.
func TestQuitIsNoOpOnOpenListConfirm(t *testing.T) {
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init board: %v", err)
	}
	t1, err := tasks.Create(dir, "First task", "", nil, nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	m := NewModel(dir)
	m.width, m.height = 80, 24
	m.view = viewList
	m.list.filtered = []*models.Task{t1}
	m.list.activePane = paneList
	m.list.confirm = confirmDelete
	m.list.confirmText = "delete task-1? [y/n]"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	um := updated.(Model)

	if um.list.confirm != confirmDelete {
		t.Errorf("expected confirm to stay pending after q, got %v", um.list.confirm)
	}
	if isQuit(cmd) {
		t.Error("q should not quit the app while a confirm is pending")
	}

	// esc still cancels.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).list.confirm != confirmNone {
		t.Error("expected esc to cancel the pending confirm")
	}
}

// TestQuitQuitsWhenNoModalOpen is the control case: q still quits normally
// when nothing is open.
func TestQuitQuitsWhenNoModalOpen(t *testing.T) {
	m := NewModel(t.TempDir())
	m.width, m.height = 80, 24

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !isQuit(cmd) {
		t.Error("expected q to quit when no modal/confirm is open")
	}
}

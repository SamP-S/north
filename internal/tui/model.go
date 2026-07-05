// Package tui provides the interactive terminal UI for North, built with
// Bubble Tea. It exposes a single entry point: NewModel.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// viewMode selects the active top-level view.
type viewMode int

const (
	viewBoard viewMode = iota
	viewList
)

// reloadMsg triggers a data reload in all sub-models.
type reloadMsg struct{}

// selectTaskMsg switches to the list view with a specific task focused.
type selectTaskMsg struct{ taskID string }

// errMsg carries a transient error string to display in the status bar.
type errMsg struct{ err error }

// Model is the root Bubble Tea model. It owns the view-mode switch, the modal
// layer, and the $EDITOR flow; navigation and rendering are delegated to
// boardModel and listModel.
type Model struct {
	boardDir string
	view     viewMode
	board    boardModel
	list     listModel
	modal    modal
	showHelp bool
	err      error
	width    int
	height   int
}

// NewModel constructs a Model for the board at boardDir.
func NewModel(boardDir string) Model {
	return Model{
		boardDir: boardDir,
		view:     viewBoard,
		board:    newBoardModel(boardDir),
		list:     newListModel(boardDir),
	}
}

// Init loads initial data for both sub-models.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.board.Init(),
		m.list.Init(),
	)
}

// searching reports whether the list view's search input is capturing keys.
func (m Model) searching() bool {
	return m.view == viewList && m.list.searching
}

// selectedTask returns the task under the cursor in the active view.
func (m Model) selectedTask() *models.Task {
	if m.view == viewBoard {
		return m.board.selectedTask()
	}
	return m.list.selected()
}

// Update handles global events (resize, quit, help, modals, action keys) and
// delegates navigation to the active sub-model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.board.setSize(m.width, m.height)
		m.list.setSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		return m.updateKey(msg)

	case reloadMsg:
		var bc, lc tea.Cmd
		m.board, bc = m.board.reload()
		m.list, lc = m.list.reload()
		return m, tea.Batch(bc, lc)

	case editorDoneMsg:
		cmd, err := m.handleEditorDone(msg)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		return m, cmd

	case errMsg:
		m.err = msg.err
		return m, nil

	// boardDataMsg must always reach the board regardless of which view is
	// active, because reloads are triggered from the list view too.
	case boardDataMsg:
		m.board = m.board.applyData(msg)
		return m, nil

	case selectTaskMsg:
		m.view = viewList
		m.list.selectByID(msg.taskID)
		m.err = nil
		return m, nil
	}

	return m.delegate(msg)
}

// updateKey routes a key press: quit/help/meta first, then the modal layer,
// then shared action keys, then the active sub-model.
func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Search mode captures every key except esc/enter (handled by the list).
	if m.searching() {
		return m.delegate(msg)
	}

	// Modal layer.
	if m.modal.open() {
		switch msg.String() {
		case "q", "?", "tab", "r":
			return m, nil // meta keys are inert while a modal is open
		}
		var cmd tea.Cmd
		m.modal, cmd = m.modal.update(msg, m.boardDir)
		return m, cmd
	}

	switch msg.String() {
	case "q":
		if m.showHelp {
			return m, nil // q only quits from the main views; esc closes help
		}
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "esc":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
	case "tab":
		if m.view == viewBoard {
			m.view = viewList
		} else {
			m.view = viewBoard
		}
		m.err = nil
		return m, nil
	case "r":
		m.err = nil
		return m, func() tea.Msg { return reloadMsg{} }
	}

	if m.showHelp {
		return m, nil // help overlay swallows the rest
	}

	// Shared action keys — identical in both views.
	switch {
	case key.Matches(msg, keys.Create):
		return m, openEditor(createTemplate(), modeCreate, "")

	case key.Matches(msg, keys.Edit):
		if t := m.selectedTask(); t != nil {
			tmpl, err := editTemplate(t)
			if err != nil {
				m.err = err
				return m, nil
			}
			return m, openEditor(tmpl, modeEdit, t.ID)
		}
		return m, nil

	case key.Matches(msg, keys.Move):
		if t := m.selectedTask(); t != nil && t.State == models.StateActive {
			m.modal = modal{mode: modalStatusPicker, cursor: statusIndex(t.Status), taskID: t.ID}
		}
		return m, nil

	case key.Matches(msg, keys.State):
		if t := m.selectedTask(); t != nil {
			m.modal = modal{mode: modalStatePicker, cursor: stateIndex(t.State), taskID: t.ID}
		}
		return m, nil

	case key.Matches(msg, keys.Delete):
		if t := m.selectedTask(); t != nil {
			m.modal = modal{mode: modalConfirmDelete, taskID: t.ID,
				confirm: deleteConfirmText(m.boardDir, t)}
		}
		return m, nil
	}

	return m.delegate(msg)
}

// delegate passes a message to the active sub-model.
func (m Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.err = nil
	}
	switch m.view {
	case viewBoard:
		var cmd tea.Cmd
		m.board, cmd = m.board.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
}

// handleEditorDone reads the temp file left by the editor, parses it, and
// calls the appropriate task operation (create or edit).
func (m *Model) handleEditorDone(msg editorDoneMsg) (tea.Cmd, error) {
	defer os.Remove(msg.tmpPath)
	if msg.canceled {
		return nil, nil
	}

	raw, err := os.ReadFile(msg.tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading editor output: %w", err)
	}
	doc, err := tasks.ParseEditorDoc(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing editor output: %w", err)
	}
	if strings.TrimSpace(doc.Title) == "" {
		return nil, fmt.Errorf("aborted: no title provided")
	}

	switch msg.mode {
	case modeCreate:
		if _, err := tasks.Create(m.boardDir, doc.Title, doc.Agent, doc.Labels, doc.DependsOn, doc.Body); err != nil {
			return nil, err
		}
	case modeEdit:
		labels := doc.Labels
		if labels == nil {
			labels = []string{}
		}
		deps := doc.DependsOn
		if deps == nil {
			deps = []string{}
		}
		if _, err := tasks.Edit(m.boardDir, msg.taskID, tasks.EditOpts{
			Title: &doc.Title, Agent: &doc.Agent,
			Labels: &labels, DependsOn: &deps, Body: &doc.Body,
		}); err != nil {
			return nil, err
		}
	}

	return func() tea.Msg { return reloadMsg{} }, nil
}

// View renders the active view, the help overlay, or the modal layer.
func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	var body string
	switch {
	case m.modal.open():
		body = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.modal.view())
	case m.showHelp:
		body = m.helpView()
	case m.view == viewBoard:
		body = m.board.View()
	default:
		body = m.list.View()
	}

	if m.err != nil {
		errLine := styleError.Render("  error: " + m.err.Error())
		return body + "\n" + errLine
	}
	return body
}

// helpView renders the help overlay.
func (m Model) helpView() string {
	rows := [][2]string{
		{"tab", "switch board ↔ list"},
		{"j/k ↑/↓", "navigate"},
		{"h/l ←/→", "columns (board) / panes (list)"},
		{"c", "create task in $EDITOR"},
		{"e", "edit task in $EDITOR"},
		{"m", "set status (active tasks only)"},
		{"s", "set state (draft/active/archive)"},
		{"d", "delete task"},
		{"r", "reload from disk"},
		{"/", "search / filter tasks"},
		{"esc", "cancel / clear search"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}

	var sb strings.Builder
	for _, row := range rows {
		sb.WriteString(styleHelpKey.Render(fmt.Sprintf("  %-16s", row[0])))
		sb.WriteString(styleHelpDesc.Render(row[1]))
		sb.WriteString("\n")
	}

	content := styleHelp.Render(
		styleHeader.Render("North TUI — keyboard shortcuts") + "\n\n" + sb.String(),
	)

	// centre in the terminal
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// stateLabel returns a short display label for a task state.
func stateLabel(s models.TaskState) string {
	switch s {
	case models.StateDraft:
		return "draft"
	case models.StateActive:
		return "active"
	case models.StateArchive:
		return "archive"
	default:
		return string(s)
	}
}

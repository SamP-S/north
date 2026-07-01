// Package tui provides the interactive terminal UI for North, built with
// Bubble Tea. It exposes a single entry point: NewModel.
package tui

import (
	"fmt"
	"os"
	"strings"

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

// Model is the root Bubble Tea model. It owns the view-mode switch and
// delegates rendering / input to boardModel or listModel.
type Model struct {
	boardDir string
	view     viewMode
	board    boardModel
	list     listModel
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

// modalOpen reports whether either sub-model has an active modal or confirm
// overlay. While one is open, q and ? are suppressed (no-op) instead of their
// global meaning (quit, toggle help) — esc is the only way to cancel a modal.
func (m Model) modalOpen() bool {
	return m.board.modal != modalNone || m.list.confirm != confirmNone || m.list.modal != modalNone
}

// Update handles global events (window resize, tab, quit, help) and delegates
// all others to the active sub-model. Shared messages (reload, editor, error)
// are also intercepted here.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.board.setSize(m.width, m.height)
		m.list.setSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			// While a modal/confirm is open, q is a no-op — esc is the only way
			// to cancel a modal, so don't quit and don't fall through to a
			// handler that would treat it as cancel either.
			if !m.list.searching && !m.modalOpen() {
				return m, tea.Quit
			}
			if m.modalOpen() {
				return m, nil
			}
		case "tab":
			if !m.list.searching {
				if m.view == viewBoard {
					m.view = viewList
				} else {
					m.view = viewBoard
				}
				m.err = nil
				return m, nil
			}
		case "?":
			if !m.list.searching && !m.modalOpen() {
				m.showHelp = !m.showHelp
				return m, nil
			}
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
		}

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

	// boardDataMsg must always reach the board regardless of which view is active,
	// because reloads are triggered from the list view too.
	case boardDataMsg:
		m.board = m.board.applyData(msg)
		return m, nil

	case selectTaskMsg:
		m.view = viewList
		m.list.selectByID(msg.taskID)
		m.err = nil
		return m, nil
	}

	// delegate to active sub-model
	m.err = nil
	switch m.view {
	case viewBoard:
		var cmd tea.Cmd
		m.board, cmd = m.board.Update(msg)
		cmds = append(cmds, cmd)
	case viewList:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleEditorDone reads the temp file left by the editor, parses it, and
// calls the appropriate task operation (create or edit).
func (m *Model) handleEditorDone(msg editorDoneMsg) (tea.Cmd, error) {
	defer os.Remove(msg.tmpPath)

	raw, err := os.ReadFile(msg.tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading editor output: %w", err)
	}
	title, body, agent, labels, dependsOn := ParseEditorResult(string(raw))
	if strings.TrimSpace(title) == "" || title == "Task title here" {
		return nil, fmt.Errorf("aborted: no title provided")
	}

	switch msg.mode {
	case modeCreate:
		if _, err := tasks.Create(m.boardDir, title, agent, labels, dependsOn, body); err != nil {
			return nil, err
		}
	case modeEdit:
		if _, err := tasks.Edit(m.boardDir, msg.taskID, &title, &agent, &labels, &dependsOn, &body); err != nil {
			return nil, err
		}
	}

	return func() tea.Msg { return reloadMsg{} }, nil
}

// View renders the active view or the help overlay.
func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	var body string
	if m.showHelp {
		body = m.helpView()
	} else {
		switch m.view {
		case viewBoard:
			body = m.board.View()
		case viewList:
			body = m.list.View()
		}
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
		{"m", "move status (active tasks only)"},
		{"p", "promote draft→active / demote active→draft"},
		{"a", "archive task / restore if already archived"},
		{"d", "delete task"},
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

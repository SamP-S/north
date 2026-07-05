// Package tui provides the interactive terminal UI for North, built with
// Bubble Tea. It exposes a single entry point: NewModel.
//
// The TUI is keyboard-only by design — no mouse support, anywhere.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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

// errMsg carries a failure into the status bar.
type errMsg struct{ err error }

// actionDoneMsg reports a completed task mutation: its notice is shown in the
// status bar and the board reloads.
type actionDoneMsg struct{ notice notice }

// noticeLevel grades a status-bar message.
type noticeLevel int

const (
	noticeNone noticeLevel = iota
	noticeSuccess
	noticeWarn
	noticeError
)

// notice is a transient status-bar message, cleared on the next keypress.
type notice struct {
	level noticeLevel
	text  string
}

// Model is the root Bubble Tea model. It owns the view-mode switch, the modal
// layer, the $EDITOR flow, the search filter, and the status bar; navigation
// and rendering are delegated to boardModel and listModel.
type Model struct {
	boardDir    string
	view        viewMode
	board       boardModel
	list        listModel
	modal       modal
	showHelp    bool
	notice      notice
	searchInput textinput.Model
	searching   bool
	query       string
	width       int
	height      int
}

// NewModel constructs a Model for the board at boardDir.
func NewModel(boardDir string) Model {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 120
	ti.Prompt = "/ "
	return Model{
		boardDir:    boardDir,
		view:        viewBoard,
		board:       newBoardModel(boardDir),
		list:        newListModel(boardDir),
		searchInput: ti,
	}
}

// Init loads initial data for both sub-models.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.board.Init(),
		m.list.Init(),
	)
}

// selectedTask returns the task under the cursor in the active view.
func (m Model) selectedTask() *models.Task {
	if m.view == viewBoard {
		return m.board.selectedTask()
	}
	return m.list.selected()
}

// setQuery propagates the search query to both sub-models.
func (m *Model) setQuery(q string) {
	m.query = q
	m.board.setFilter(q)
	m.list.setFilter(q)
}

// clearFilter drops the active search filter everywhere.
func (m *Model) clearFilter() {
	m.searchInput.SetValue("")
	m.setQuery("")
}

// Update handles global events (resize, quit, help, modals, search, action
// keys) and delegates navigation to the active sub-model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Root-owned lines: the status bar plus the footer, which wraps to
		// extra lines on narrow terminals rather than overflowing.
		bodyH := m.height - 1 - m.footerHeight()
		m.board.setSize(m.width, bodyH)
		m.list.setSize(m.width, bodyH)
		return m, nil

	case tea.KeyMsg:
		return m.updateKey(msg)

	case reloadMsg:
		var bc, lc tea.Cmd
		m.board, bc = m.board.reload()
		m.list, lc = m.list.reload()
		return m, tea.Batch(bc, lc)

	case actionDoneMsg:
		m.notice = msg.notice
		return m, func() tea.Msg { return reloadMsg{} }

	case editorDoneMsg:
		cmd, err := m.handleEditorDone(msg)
		if err != nil {
			m.notice = notice{noticeError, err.Error()}
			return m, nil
		}
		return m, cmd

	case errMsg:
		m.notice = notice{noticeError, msg.err.Error()}
		return m, nil

	// boardDataMsg must always reach the board regardless of which view is
	// active, because reloads are triggered from the list view too.
	case boardDataMsg:
		m.board = m.board.applyData(msg)
		return m, nil

	case selectTaskMsg:
		m.view = viewList
		if !m.list.selectByID(msg.taskID) && m.query != "" {
			// The target is hidden by the filter — clear it so the
			// selection can never silently fail.
			m.clearFilter()
			m.list.selectByID(msg.taskID)
		}
		return m, nil
	}

	return m.delegate(msg)
}

// updateKey routes a key press: quit/help/meta first, then search input, then
// the modal layer, then shared action keys, then the active sub-model.
func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	m.notice = notice{} // any keypress clears the status-bar message

	// Search input captures every key except esc/enter.
	if m.searching {
		switch msg.String() {
		case "esc", "enter":
			m.searching = false
			m.searchInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.setQuery(m.searchInput.Value())
		return m, cmd
	}

	// Modal layer.
	if m.modal.open() {
		switch msg.String() {
		case "q", "?", "tab", "r", "/":
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
		// Esc clears the filter, except when the list's detail pane is
		// focused (there it steps back to the list pane first).
		if m.query != "" && (m.view == viewBoard || m.list.activePane == paneList) {
			m.clearFilter()
			return m, nil
		}
	case "tab":
		if m.view == viewBoard {
			m.view = viewList
		} else {
			m.view = viewBoard
		}
		return m, nil
	case "r":
		return m, func() tea.Msg { return reloadMsg{} }
	}

	if m.showHelp {
		return m, nil // help overlay swallows the rest
	}

	// Enter on a board card opens the read-only task popup in place (the
	// list view keeps enter for focusing the detail pane).
	if m.view == viewBoard && key.Matches(msg, keys.Enter) {
		if t := m.board.selectedTask(); t != nil {
			vpW := max(20, m.width-14)
			vpH := max(5, m.height-8)
			vp := viewport.New(vpW, vpH)
			vp.SetContent(m.list.renderDetail(t))
			m.modal = modal{mode: modalTaskView, taskID: t.ID, taskState: t.State, vp: vp}
		}
		return m, nil
	}

	// Shared action keys — identical in both views.
	switch {
	case key.Matches(msg, keys.Search):
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.Create):
		return m, openEditor(createTemplate(), modeCreate, "")

	case key.Matches(msg, keys.Edit):
		if t := m.selectedTask(); t != nil {
			tmpl, err := editTemplate(t)
			if err != nil {
				m.notice = notice{noticeError, err.Error()}
				return m, nil
			}
			return m, openEditor(tmpl, modeEdit, t.ID)
		}
		return m, nil

	case key.Matches(msg, keys.Move):
		if t := m.selectedTask(); t != nil {
			m.modal = modal{mode: modalStatusPicker, cursor: statusIndex(t.Status),
				taskID: t.ID, taskState: t.State}
		}
		return m, nil

	case key.Matches(msg, keys.State):
		if t := m.selectedTask(); t != nil {
			m.modal = modal{mode: modalStatePicker, cursor: stateIndex(t.State),
				taskID: t.ID, taskState: t.State}
		}
		return m, nil

	case key.Matches(msg, keys.Delete):
		if t := m.selectedTask(); t != nil {
			m.modal = modal{mode: modalConfirmDelete, taskID: t.ID, taskState: t.State,
				confirm: deleteConfirmText(m.boardDir, t)}
		}
		return m, nil
	}

	return m.delegate(msg)
}

// delegate passes a message to the active sub-model.
func (m Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		t, err := tasks.Create(m.boardDir, doc.Title, doc.Assignee, doc.Labels, doc.DependsOn, doc.Body)
		if err != nil {
			return nil, err
		}
		m.notice = notice{noticeSuccess, fmt.Sprintf("created %s: %s", t.ID, t.Title)}
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
			Title: &doc.Title, Assignee: &doc.Assignee,
			Labels: &labels, DependsOn: &deps, Body: &doc.Body,
		}); err != nil {
			return nil, err
		}
		m.notice = notice{noticeSuccess, fmt.Sprintf("edited %s", msg.taskID)}
	}

	return func() tea.Msg { return reloadMsg{} }, nil
}

// statusLine renders the root-owned line above the footer: the search input
// while typing, then any notice, then a persistent filter indicator.
func (m Model) statusLine() string {
	switch {
	case m.searching:
		return "  " + m.searchInput.View()
	case m.notice.level != noticeNone:
		return noticeStyle(m.notice.level).Render("  " + m.notice.text)
	case m.query != "":
		return styleFooter.Render(fmt.Sprintf("  filter: %q (esc clears, / edits)", m.query))
	}
	return ""
}

// View renders the active view, the help overlay, or the modal layer.
func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	switch {
	case m.modal.open():
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.modal.view())
	case m.showHelp:
		return m.helpView()
	}

	var body string
	if m.view == viewBoard {
		body = m.board.View()
	} else {
		body = m.list.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.statusLine(), m.footer())
}

// Footer hint lines for each view.
const (
	boardHints = "↵ view  c create  e edit  m status  s state  d delete  r reload  / filter  tab→list  ? help  q quit"
	listHints  = "j/k navigate  g/G top/bottom  ←/→ panes  c create  e edit  m status  s state  d delete  r reload  / filter  tab→board  ? help  q quit"
)

// footer renders the active view's key-hint bar, wrapped to the terminal
// width (extra lines on narrow terminals instead of overflow).
func (m Model) footer() string {
	hints := boardHints
	warnings := m.board.warnings
	if m.view == viewList {
		hints = listHints
		warnings = m.list.warnings
	}
	if warnings > 0 {
		hints = fmt.Sprintf("⚠ %d file warning(s)  ", warnings) + hints
	}
	return styleFooter.Width(m.width).Render("  " + hints)
}

// footerHeight returns the line count the footer needs at the current width —
// the max across both views so the body height is stable when tabbing.
func (m Model) footerHeight() int {
	h := lipgloss.Height(styleFooter.Width(m.width).Render("  " + boardHints))
	if lh := lipgloss.Height(styleFooter.Width(m.width).Render("  " + listHints)); lh > h {
		h = lh
	}
	return h
}

// helpView renders the help overlay.
func (m Model) helpView() string {
	rows := [][2]string{
		{"tab", "switch board ↔ list"},
		{"j/k ↑/↓", "navigate"},
		{"h/l ←/→", "columns (board) / panes (list)"},
		{"g/G", "jump to top / bottom"},
		{"enter", "view task (board) / focus pane (list)"},
		{"c", "create task in $EDITOR"},
		{"e", "edit task in $EDITOR"},
		{"m", "set status"},
		{"s", "set state (draft/active/archive)"},
		{"d", "delete task"},
		{"r", "reload from disk"},
		{"/", "filter tasks (board & list)"},
		{"esc", "cancel / clear filter"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}

	tags := [][2]string{
		{"●", "status colour (draft/archive columns)"},
		{"@", "assignee set (person or agent)"},
		{"!", "waiting — unmet dependency (resolves when done or archived)"},
		{"&", "other tasks depend on it"},
	}

	var sb strings.Builder
	for _, row := range rows {
		sb.WriteString(styleHelpKey.Render(fmt.Sprintf("  %-16s", row[0])))
		sb.WriteString(styleHelpDesc.Render(row[1]))
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + styleHeader.Render("Card tags") + "\n\n")
	for _, row := range tags {
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

// board.go — kanban board sub-model for the North TUI.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// modalMode identifies the active overlay modal.
type modalMode int

const (
	modalNone         modalMode = iota
	modalStatusPicker           // status-move picker
	modalConfirm                // archive/delete confirmation
)

// confirmKind identifies which destructive action awaits confirmation.
type confirmKind int

const (
	confirmNone    confirmKind = iota
	confirmArchive             // archive a task
	confirmDelete              // delete a task
)

// boardDataMsg carries freshly loaded board data back into the model.
type boardDataMsg struct {
	cols         []boardColumn
	draftCount   int
	archiveCount int
}

// boardColumn holds tasks for one status column together with the cursor.
type boardColumn struct {
	status string
	tasks  []*models.Task
	cursor int
}

// boardModel is the kanban board sub-model. It does not implement tea.Model
// directly; the root Model delegates to it.
type boardModel struct {
	boardDir     string
	columns      []boardColumn
	colIdx       int
	draftCount   int
	archiveCount int
	width        int
	height       int

	// modal state
	modal       modalMode
	modalCursor int         // cursor position inside the status-picker list
	pending     confirmKind // which action awaits confirmation
	pendingID   string      // task ID pending confirmation
	pendingText string      // message shown in the confirm modal
}

// newBoardModel constructs an empty boardModel for boardDir.
func newBoardModel(boardDir string) boardModel {
	return boardModel{boardDir: boardDir}
}

// Init loads the initial board data.
func (m boardModel) Init() tea.Cmd {
	dir := m.boardDir
	return func() tea.Msg {
		cols, draftCount, archiveCount, err := loadData(dir)
		if err != nil {
			return errMsg{err}
		}
		return boardDataMsg{cols: cols, draftCount: draftCount, archiveCount: archiveCount}
	}
}

// setSize records the terminal dimensions. Uses a pointer receiver so the
// assignment in Model.Update takes effect without capturing a return value.
func (m *boardModel) setSize(width, height int) {
	m.width = width
	m.height = height
}

// reload returns the current model and a command to reload data from disk,
// preserving cursor positions.
func (m boardModel) reload() (boardModel, tea.Cmd) {
	return m, m.Init()
}

// Update handles messages for the board sub-model.
func (m boardModel) Update(msg tea.Msg) (boardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case boardDataMsg:
		return m.applyData(msg), nil
	case tea.KeyMsg:
		if m.modal != modalNone {
			return m.updateModal(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

// View renders the board or an active modal overlay.
func (m boardModel) View() string {
	if m.width == 0 {
		return "loading…"
	}

	switch m.modal {
	case modalStatusPicker:
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.renderStatusPicker())
	case modalConfirm:
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.renderConfirm())
	}

	return m.renderBoard()
}

// ─── data loading ────────────────────────────────────────────────────────────

// loadData fetches all active tasks (grouped by status in models.Statuses
// order) plus draft and archive counts from the board directory.
func loadData(boardDir string) ([]boardColumn, int, int, error) {
	active, err := tasks.List(boardDir, []models.TaskState{models.StateActive}, "")
	if err != nil {
		return nil, 0, 0, err
	}

	byStatus := make(map[string][]*models.Task, len(models.Statuses))
	for _, t := range active {
		s := string(t.Status)
		byStatus[s] = append(byStatus[s], t)
	}

	cols := make([]boardColumn, len(models.Statuses))
	for i, s := range models.Statuses {
		cols[i] = boardColumn{status: string(s), tasks: byStatus[string(s)]}
	}

	draftCount, err := tasks.StateCount(boardDir, models.StateDraft)
	if err != nil {
		return nil, 0, 0, err
	}
	archiveCount, err := tasks.StateCount(boardDir, models.StateArchive)
	if err != nil {
		return nil, 0, 0, err
	}

	return cols, draftCount, archiveCount, nil
}

// applyData applies a boardDataMsg while preserving column/cursor positions.
func (m boardModel) applyData(msg boardDataMsg) boardModel {
	old := m.columns
	m.columns = msg.cols
	m.draftCount = msg.draftCount
	m.archiveCount = msg.archiveCount

	// Clamp the column index.
	if n := len(m.columns); m.colIdx >= n {
		if n > 0 {
			m.colIdx = n - 1
		} else {
			m.colIdx = 0
		}
	}

	// Restore and clamp per-column cursor positions.
	for i := range m.columns {
		prev := 0
		if i < len(old) {
			prev = old[i].cursor
		}
		if n := len(m.columns[i].tasks); prev >= n {
			if n > 0 {
				prev = n - 1
			} else {
				prev = 0
			}
		}
		m.columns[i].cursor = prev
	}
	return m
}

// selectedTask returns the task under the cursor, or nil if none is available.
func (m boardModel) selectedTask() *models.Task {
	if len(m.columns) == 0 {
		return nil
	}
	col := m.columns[m.colIdx]
	if len(col.tasks) == 0 || col.cursor >= len(col.tasks) {
		return nil
	}
	return col.tasks[col.cursor]
}

// ─── key handling ────────────────────────────────────────────────────────────

func (m boardModel) updateKeys(msg tea.KeyMsg) (boardModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if len(m.columns) > 0 {
			col := m.columns[m.colIdx]
			if col.cursor > 0 {
				col.cursor--
				m.columns[m.colIdx] = col
			}
		}

	case key.Matches(msg, keys.Down):
		if len(m.columns) > 0 {
			col := m.columns[m.colIdx]
			if col.cursor < len(col.tasks)-1 {
				col.cursor++
				m.columns[m.colIdx] = col
			}
		}

	case key.Matches(msg, keys.Left):
		if m.colIdx > 0 {
			m.colIdx--
			m.clampCursor()
		}

	case key.Matches(msg, keys.Right):
		if m.colIdx < len(m.columns)-1 {
			m.colIdx++
			m.clampCursor()
		}

	case key.Matches(msg, keys.Enter):
		if t := m.selectedTask(); t != nil {
			return m, func() tea.Msg { return selectTaskMsg{taskID: t.ID} }
		}

	case key.Matches(msg, keys.Create):
		return m, openEditor(createTemplate(), modeCreate, "")

	case key.Matches(msg, keys.Edit):
		if t := m.selectedTask(); t != nil {
			return m, openEditor(taskToTemplate(t), modeEdit, t.ID)
		}

	case key.Matches(msg, keys.Move):
		if t := m.selectedTask(); t != nil && t.State == models.StateActive {
			m.modal = modalStatusPicker
			m.modalCursor = m.statusIndex(t.Status)
		}

	case key.Matches(msg, keys.Promote):
		return m.doPromote()

	case key.Matches(msg, keys.Archive):
		if t := m.selectedTask(); t != nil {
			m.modal = modalConfirm
			m.pending = confirmArchive
			m.pendingID = t.ID
			m.pendingText = fmt.Sprintf("archive %s? [y/n]", t.ID)
		}

	case key.Matches(msg, keys.Delete):
		if t := m.selectedTask(); t != nil {
			m.modal = modalConfirm
			m.pending = confirmDelete
			m.pendingID = t.ID
			m.pendingText = fmt.Sprintf("delete %s? [y/n]", t.ID)
		}
	}
	return m, nil
}

// clampCursor ensures the cursor in the current column is within bounds after
// a column switch.
func (m *boardModel) clampCursor() {
	if len(m.columns) == 0 {
		return
	}
	col := m.columns[m.colIdx]
	if n := len(col.tasks); col.cursor >= n {
		if n > 0 {
			col.cursor = n - 1
		} else {
			col.cursor = 0
		}
		m.columns[m.colIdx] = col
	}
}

// statusIndex returns the index of s in models.Statuses, defaulting to 0.
func (m boardModel) statusIndex(s models.TaskStatus) int {
	for i, st := range models.Statuses {
		if st == s {
			return i
		}
	}
	return 0
}

// doPromote promotes a draft task or demotes an active task.
func (m boardModel) doPromote() (boardModel, tea.Cmd) {
	t := m.selectedTask()
	if t == nil {
		return m, nil
	}
	boardDir := m.boardDir
	taskID := t.ID
	state := t.State
	return m, func() tea.Msg {
		var err error
		switch state {
		case models.StateDraft:
			_, err = tasks.Promote(boardDir, taskID)
		case models.StateActive:
			_, err = tasks.Demote(boardDir, taskID)
		default:
			return nil
		}
		if err != nil {
			return errMsg{err}
		}
		return reloadMsg{}
	}
}

// ─── modal handling ──────────────────────────────────────────────────────────

func (m boardModel) updateModal(msg tea.KeyMsg) (boardModel, tea.Cmd) {
	switch m.modal {
	case modalStatusPicker:
		return m.updateStatusPicker(msg)
	case modalConfirm:
		return m.updateConfirm(msg)
	}
	return m, nil
}

func (m boardModel) updateStatusPicker(msg tea.KeyMsg) (boardModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.modalCursor > 0 {
			m.modalCursor--
		}

	case key.Matches(msg, keys.Down):
		if m.modalCursor < len(models.Statuses)-1 {
			m.modalCursor++
		}

	case key.Matches(msg, keys.Enter):
		t := m.selectedTask()
		if t == nil {
			m.modal = modalNone
			return m, nil
		}
		newStatus := string(models.Statuses[m.modalCursor])
		boardDir := m.boardDir
		taskID := t.ID
		m.modal = modalNone
		return m, func() tea.Msg {
			if _, err := tasks.SetStatus(boardDir, taskID, newStatus); err != nil {
				return errMsg{err}
			}
			return reloadMsg{}
		}

	case key.Matches(msg, keys.Esc):
		m.modal = modalNone
	}
	return m, nil
}

func (m boardModel) updateConfirm(msg tea.KeyMsg) (boardModel, tea.Cmd) {
	switch {
	case msg.String() == "y" || msg.String() == "Y":
		taskID := m.pendingID
		kind := m.pending
		boardDir := m.boardDir
		m.modal = modalNone
		m.pending = confirmNone
		m.pendingID = ""
		m.pendingText = ""
		return m, func() tea.Msg {
			var err error
			switch kind {
			case confirmArchive:
				_, err = tasks.Archive(boardDir, taskID)
			case confirmDelete:
				err = tasks.Delete(boardDir, taskID)
			}
			if err != nil {
				return errMsg{err}
			}
			return reloadMsg{}
		}

	case msg.String() == "n" || msg.String() == "N" || key.Matches(msg, keys.Esc):
		m.modal = modalNone
		m.pending = confirmNone
		m.pendingID = ""
		m.pendingText = ""
	}
	return m, nil
}

// ─── rendering ───────────────────────────────────────────────────────────────

func (m boardModel) renderBoard() string {
	if len(m.columns) == 0 {
		return "loading…"
	}

	n := len(m.columns)

	// Each column's outer width (border included); last column absorbs remainder.
	colW := m.width / n
	innerW := colW - 2
	if innerW < 4 {
		innerW = 4
	}

	// Leave 1 line for the footer.
	colH := m.height - 1
	innerH := colH - 2
	if innerH < 1 {
		innerH = 1
	}

	rendered := make([]string, n)
	for i, col := range m.columns {
		w := innerW
		if i == n-1 {
			// Last column absorbs any width left by integer division.
			used := colW * (n - 1)
			w = m.width - used - 2
			if w < 4 {
				w = 4
			}
		}
		rendered[i] = m.renderColumn(i, col, w, innerH)
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	return board + "\n" + m.renderFooter()
}

func (m boardModel) renderFooter() string {
	info := fmt.Sprintf("  drafts: %d  archive: %d", m.draftCount, m.archiveCount)
	hints := "↵ view  c create  e edit  m move  p promote/demote/restore  a archive  d delete  tab→list  ? help  q quit"

	infoW := lipgloss.Width(info)
	hintsW := lipgloss.Width(hints)
	gap := m.width - infoW - hintsW - 2
	if gap < 1 {
		gap = 1
	}

	return styleFooter.Render(info + strings.Repeat(" ", gap) + hints)
}

func (m boardModel) renderColumn(idx int, col boardColumn, innerW, innerH int) string {
	isActive := idx == m.colIdx

	// Header: coloured status label + task count.
	header := statusStyle(col.status).Render(col.status) +
		styleID.Render(fmt.Sprintf(" (%d)", len(col.tasks)))

	lines := []string{header, ""}

	if len(col.tasks) == 0 {
		lines = append(lines, styleID.Render("(empty)"))
	} else {
		for j, t := range col.tasks {
			// Truncate the title to avoid wrapping (rough visible-length estimate).
			maxTitle := innerW - len(t.ID) - 2
			if maxTitle < 0 {
				maxTitle = 0
			}
			title := t.Title
			if len(title) > maxTitle {
				title = title[:maxTitle]
			}
			line := styleID.Render(t.ID) + " " + title

			if isActive && j == col.cursor {
				lines = append(lines, styleCardSelected.Width(innerW).Render(line))
			} else {
				lines = append(lines, styleCardNormal.Width(innerW).Render(line))
			}
		}
	}

	content := strings.Join(lines, "\n")

	colStyle := styleColumnInactive
	if isActive {
		colStyle = styleColumnActive
	}
	return colStyle.Width(innerW).Height(innerH).Render(content)
}

func (m boardModel) renderStatusPicker() string {
	var sb strings.Builder
	sb.WriteString(styleHeader.Render("move to status") + "\n\n")
	for i, s := range models.Statuses {
		prefix := "  "
		if i == m.modalCursor {
			prefix = "> "
		}
		sb.WriteString(prefix + statusStyle(string(s)).Render(string(s)) + "\n")
	}
	return styleModal.Render(strings.TrimRight(sb.String(), "\n"))
}

func (m boardModel) renderConfirm() string {
	return styleModal.Render(m.pendingText)
}

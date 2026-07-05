// board.go — kanban board sub-model for the North TUI.
//
// The board holds navigation state only; action keys (c/e/m/s/d), search, and
// every modal live in the root Model so both views behave identically.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// boardDataMsg carries freshly loaded board data back into the model.
type boardDataMsg struct {
	cols         []boardColumn
	draftCount   int
	archiveCount int
	warnings     int
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
	all          []boardColumn // unfiltered, as loaded
	columns      []boardColumn // filter applied
	filter       string
	colIdx       int
	draftCount   int
	archiveCount int
	warnings     int
	width        int
	height       int
}

// newBoardModel constructs an empty boardModel for boardDir.
func newBoardModel(boardDir string) boardModel {
	return boardModel{boardDir: boardDir}
}

// Init loads the initial board data.
func (m boardModel) Init() tea.Cmd {
	dir := m.boardDir
	return func() tea.Msg {
		data, err := loadData(dir)
		if err != nil {
			return errMsg{err}
		}
		return data
	}
}

// setSize records the terminal dimensions. Uses a pointer receiver so the
// assignment in Model.Update takes effect without capturing a return value.
func (m *boardModel) setSize(width, height int) {
	m.width = width
	m.height = height
}

// setFilter applies a search query to the columns, preserving cursors.
func (m *boardModel) setFilter(q string) {
	m.filter = q
	m.rebuild()
}

// reload returns the current model and a command to reload data from disk,
// preserving cursor positions.
func (m boardModel) reload() (boardModel, tea.Cmd) {
	return m, m.Init()
}

// Update handles messages for the board sub-model (navigation only).
func (m boardModel) Update(msg tea.Msg) (boardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case boardDataMsg:
		return m.applyData(msg), nil
	case tea.KeyMsg:
		return m.updateKeys(msg)
	}
	return m, nil
}

// View renders the board (columns only; the root owns status bar and footer).
func (m boardModel) View() string {
	if m.width == 0 {
		return "loading…"
	}
	return m.renderBoard()
}

// ─── data loading ────────────────────────────────────────────────────────────

// loadData fetches all active tasks (grouped by status in models.Statuses
// order) plus draft/archive counts and the warning tally, in one snapshot.
func loadData(boardDir string) (boardDataMsg, error) {
	snap, err := tasks.Load(boardDir)
	if err != nil {
		return boardDataMsg{}, err
	}
	active := snap.Filter([]models.TaskState{models.StateActive}, "")

	byStatus := make(map[string][]*models.Task, len(models.Statuses))
	for _, t := range active {
		s := string(t.Status)
		byStatus[s] = append(byStatus[s], t)
	}

	cols := make([]boardColumn, len(models.Statuses))
	for i, s := range models.Statuses {
		cols[i] = boardColumn{status: string(s), tasks: byStatus[string(s)]}
	}

	return boardDataMsg{
		cols:         cols,
		draftCount:   snap.StateCount(models.StateDraft),
		archiveCount: snap.StateCount(models.StateArchive),
		warnings:     len(snap.Warnings),
	}, nil
}

// applyData stores fresh board data and rebuilds the filtered columns while
// preserving column/cursor positions.
func (m boardModel) applyData(msg boardDataMsg) boardModel {
	m.all = msg.cols
	m.draftCount = msg.draftCount
	m.archiveCount = msg.archiveCount
	m.warnings = msg.warnings
	m.rebuild()
	return m
}

// rebuild recomputes the filtered columns from the loaded data, keeping the
// previous cursor positions where possible.
func (m *boardModel) rebuild() {
	old := m.columns
	m.columns = make([]boardColumn, len(m.all))
	for i, col := range m.all {
		filtered := col
		if m.filter != "" {
			filtered.tasks = nil
			for _, t := range col.tasks {
				if matchesFilter(t, m.filter) {
					filtered.tasks = append(filtered.tasks, t)
				}
			}
		}
		if i < len(old) {
			filtered.cursor = old[i].cursor
		}
		if n := len(filtered.tasks); filtered.cursor >= n {
			if n > 0 {
				filtered.cursor = n - 1
			} else {
				filtered.cursor = 0
			}
		}
		m.columns[i] = filtered
	}
	if n := len(m.columns); m.colIdx >= n && n > 0 {
		m.colIdx = n - 1
	}
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

	case key.Matches(msg, keys.Top):
		if len(m.columns) > 0 {
			m.columns[m.colIdx].cursor = 0
		}

	case key.Matches(msg, keys.Bottom):
		if len(m.columns) > 0 {
			col := m.columns[m.colIdx]
			if n := len(col.tasks); n > 0 {
				m.columns[m.colIdx].cursor = n - 1
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

// ─── rendering ───────────────────────────────────────────────────────────────

func (m boardModel) renderBoard() string {
	if len(m.columns) == 0 {
		return "loading…"
	}

	// Empty-board hint (only when nothing is filtered away).
	total := 0
	for _, col := range m.columns {
		total += len(col.tasks)
	}
	if total == 0 {
		hint := "no active tasks — press c to create, tab for the full list"
		if m.filter != "" {
			hint = "no tasks match the filter — esc clears it"
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleFooter.Render(hint))
	}

	n := len(m.columns)

	// Each column's outer width (border included); last column absorbs remainder.
	colW := m.width / n
	innerW := colW - 2
	if innerW < 4 {
		innerW = 4
	}

	colH := m.height
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

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

// renderFooter renders the one-line info + key-hint bar (composed by the root).
func (m boardModel) renderFooter() string {
	info := fmt.Sprintf("  drafts: %d  archive: %d", m.draftCount, m.archiveCount)
	if m.warnings > 0 {
		info += fmt.Sprintf("  ⚠ %d file warning(s)", m.warnings)
	}
	hints := "↵ view  c create  e edit  m status  s state  d delete  r reload  / filter  tab→list  ? help  q quit"

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

	// Cards area: window the tasks so the cursor is always visible.
	visH := innerH - len(lines)
	if visH < 1 {
		visH = 1
	}

	if len(col.tasks) == 0 {
		lines = append(lines, styleID.Render("(empty)"))
	} else {
		offset := 0
		if col.cursor >= visH {
			offset = col.cursor - visH + 1
		}
		for j := offset; j < len(col.tasks) && j < offset+visH; j++ {
			t := col.tasks[j]
			// Truncate by display width so multibyte/wide titles never break.
			// Width budget: 2-char cursor prefix + id + separating space.
			maxTitle := innerW - lipgloss.Width(t.ID) - 4
			if maxTitle < 1 {
				maxTitle = 1
			}
			title := ansi.Truncate(t.Title, maxTitle, "…")

			if isActive && j == col.cursor {
				// Each segment is bolded on its own: a styled segment ends
				// with an ANSI reset that would cancel one outer bold.
				line := styleCardSelected.Render("► ") +
					styleID.Bold(true).Render(t.ID) +
					styleCardSelected.Render(" "+title)
				lines = append(lines, styleCardNormal.Width(innerW).Render(line))
			} else {
				line := "  " + styleID.Render(t.ID) + " " + title
				lines = append(lines, styleCardNormal.Width(innerW).Render(line))
			}
		}
		if offset > 0 {
			// The spacer line doubles as a scrolled-up indicator.
			lines[1] = styleID.Render(fmt.Sprintf("↑ %d more", offset))
		}
	}

	content := strings.Join(lines, "\n")

	colStyle := styleColumnInactive
	if isActive {
		colStyle = styleColumnActive
	}
	return colStyle.Width(innerW).Height(innerH).Render(content)
}

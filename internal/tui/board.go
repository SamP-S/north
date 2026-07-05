// board.go — kanban board sub-model for the North TUI.
//
// The board holds navigation state only; action keys (c/e/m/s/d) and every
// modal live in the root Model so both views behave identically.
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
	columns      []boardColumn
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

// View renders the board.
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

// applyData applies a boardDataMsg while preserving column/cursor positions.
func (m boardModel) applyData(msg boardDataMsg) boardModel {
	old := m.columns
	m.columns = msg.cols
	m.draftCount = msg.draftCount
	m.archiveCount = msg.archiveCount
	m.warnings = msg.warnings

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
	if m.warnings > 0 {
		info += fmt.Sprintf("  ⚠ %d file warning(s)", m.warnings)
	}
	hints := "↵ view  c create  e edit  m status  s state  d delete  r reload  tab→list  ? help  q quit"

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
			// Truncate by display width so multibyte/wide titles never break.
			maxTitle := innerW - lipgloss.Width(t.ID) - 2
			if maxTitle < 1 {
				maxTitle = 1
			}
			title := ansi.Truncate(t.Title, maxTitle, "…")
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

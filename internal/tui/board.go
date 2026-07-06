// board.go — kanban board sub-model for the North TUI.
//
// The board renders the whole two-axis model on one screen: a draft column on
// the far left, the five status columns for active tasks in flow order, and
// an archive column on the far right. A task enters left, flows right.
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
	cols     []boardColumn
	tags     map[string]string // task id → card tag cluster ("@!&")
	warnings int
}

// boardColumn holds tasks for one column together with the cursor. Exactly
// one of state/status is set: the draft and archive columns are state
// columns, the five in between are status columns (active tasks).
type boardColumn struct {
	title  string
	state  models.TaskState // set for the draft/archive columns
	status string           // set for the status columns
	tasks  []*models.Task
	cursor int
}

// boardModel is the kanban board sub-model. It does not implement tea.Model
// directly; the root Model delegates to it.
type boardModel struct {
	boardDir string
	all      []boardColumn // unfiltered, as loaded
	columns  []boardColumn // filter applied
	tags     map[string]string
	filter   string
	sortKey  tasks.SortKey
	sortDesc bool
	colIdx   int
	warnings int
	width    int
	height   int
}

// newBoardModel constructs an empty boardModel for boardDir.
func newBoardModel(boardDir string) boardModel {
	return boardModel{boardDir: boardDir, sortKey: tasks.SortID, sortDesc: true}
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

// setSort changes the column ordering.
func (m *boardModel) setSort(key tasks.SortKey, desc bool) {
	m.sortKey, m.sortDesc = key, desc
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

// loadData builds the seven board columns in one snapshot. Ordering is
// applied later, in rebuild, under the model's current sort.
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

	cols := make([]boardColumn, 0, len(models.Statuses)+2)
	cols = append(cols, boardColumn{
		title: "draft", state: models.StateDraft,
		tasks: snap.Filter([]models.TaskState{models.StateDraft}, ""),
	})
	for _, s := range models.Statuses {
		cols = append(cols, boardColumn{
			title: string(s), status: string(s), tasks: byStatus[string(s)],
		})
	}
	cols = append(cols, boardColumn{
		title: "archive", state: models.StateArchive,
		tasks: snap.Filter([]models.TaskState{models.StateArchive}, ""),
	})

	return boardDataMsg{cols: cols, tags: cardTags(snap), warnings: len(snap.Warnings)}, nil
}

// cardTags computes each task's card tag cluster:
//
//	@  an assignee is set (a person or an agent)
//	!  waiting — some depends_on target is missing or unresolved
//	   (a dependency resolves when it is done or archived)
//	&  other tasks depend on this one
func cardTags(snap *tasks.Snapshot) map[string]string {
	all := snap.Filter(nil, "")
	byID := make(map[string]*models.Task, len(all))
	hasDependents := make(map[string]bool)
	for _, t := range all {
		byID[t.ID] = t
	}
	for _, t := range all {
		for _, d := range t.DependsOn {
			hasDependents[d] = true
		}
	}

	tags := make(map[string]string, len(all))
	for _, t := range all {
		var b strings.Builder
		if t.Assignee != "" {
			b.WriteByte('@')
		}
		for _, d := range t.DependsOn {
			dt, ok := byID[d]
			if !ok || (dt.Status != models.Done && dt.State != models.StateArchive) {
				b.WriteByte('!')
				break
			}
		}
		if hasDependents[t.ID] {
			b.WriteByte('&')
		}
		if b.Len() > 0 {
			tags[t.ID] = b.String()
		}
	}
	return tags
}

// applyData stores fresh board data and rebuilds the filtered columns while
// preserving column/cursor positions.
func (m boardModel) applyData(msg boardDataMsg) boardModel {
	m.all = msg.cols
	m.tags = msg.tags
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
		filtered.tasks = make([]*models.Task, 0, len(col.tasks))
		for _, t := range col.tasks {
			if m.filter == "" || matchesFilter(t, m.filter) {
				filtered.tasks = append(filtered.tasks, t)
			}
		}
		tasks.Sort(filtered.tasks, m.sortKey, m.sortDesc)
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
		hint := "no tasks — press c to create one"
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

func (m boardModel) renderColumn(idx int, col boardColumn, innerW, innerH int) string {
	isActive := idx == m.colIdx
	isStateCol := col.state != ""

	// Header: status columns in their status colour, state columns dimmed.
	var label string
	if isStateCol {
		label = stateStyle(col.state).Render(col.title)
	} else {
		label = statusStyle(col.status).Render(col.title)
	}
	header := label + styleID.Render(fmt.Sprintf(" (%d)", len(col.tasks)))

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
			selected := isActive && j == col.cursor

			// State columns carry a status-coloured dot: the column no
			// longer implies the status, so the dot shows it at a glance.
			dot := ""
			dotW := 0
			if isStateCol {
				dot = statusStyle(string(t.Status)).Bold(selected).Render("●") + " "
				dotW = 2
			}

			// Dimmed tag cluster (@ assignee, ! waiting, & has dependents).
			tagSeg := ""
			tagW := 0
			if tags := m.tags[t.ID]; tags != "" {
				tagSeg = styleID.Bold(selected).Render(tags) + " "
				tagW = len(tags) + 1
			}

			// Truncate by display width so multibyte/wide titles never
			// break. Width budget: 2-char cursor prefix + dot + id + space
			// + tags.
			maxTitle := innerW - lipgloss.Width(t.ID) - 4 - dotW - tagW
			if maxTitle < 1 {
				maxTitle = 1
			}
			title := ansi.Truncate(t.Title, maxTitle, "…")

			var line string
			if selected {
				// Each segment is bolded on its own: a styled segment ends
				// with an ANSI reset that would cancel one outer bold.
				line = styleCardSelected.Render("► ") + dot +
					styleID.Bold(true).Render(t.ID) + " " + tagSeg +
					styleCardSelected.Render(title)
			} else {
				line = "  " + dot + styleID.Render(t.ID) + " " + tagSeg + title
			}
			lines = append(lines, styleCardNormal.Width(innerW).Render(line))
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

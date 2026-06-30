// Package tui provides the interactive terminal UI for North.
package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// paneMode identifies which pane is focused in the list view.
type paneMode int

const (
	paneList paneMode = iota
	paneDetail
)

// listModel is the list+detail sub-model for the list view.
// It does not implement tea.Model directly; the root Model wraps it.
type listModel struct {
	boardDir    string
	allTasks    []*models.Task
	filtered    []*models.Task
	cursor      int
	activePane  paneMode
	vp          viewport.Model
	searchInput textinput.Model
	searching   bool
	searchQuery string
	confirm     confirmKind // reuses the confirmKind/confirmNone/… declared in board.go
	pendingFn   func() error
	width       int
	height      int
}

// newListModel constructs a zeroed listModel for the given board directory.
func newListModel(boardDir string) listModel {
	ti := textinput.New()
	ti.Placeholder = "search…"
	ti.CharLimit = 120

	vp := viewport.New(0, 0)

	return listModel{
		boardDir:    boardDir,
		searchInput: ti,
		vp:          vp,
	}
}

// Init returns a command that triggers the initial data load via reloadMsg so
// the root model's reloadMsg handler populates both sub-models together.
func (m listModel) Init() tea.Cmd {
	return func() tea.Msg { return reloadMsg{} }
}

// reload loads all tasks from disk, applies the current filter, clamps the
// cursor, and refreshes the detail viewport. Called by the root model on every
// reloadMsg.
func (m listModel) reload() (listModel, tea.Cmd) {
	ts, err := tasks.List(m.boardDir, models.StateOrder, "")
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}
	sortDescByID(ts)
	m.allTasks = ts
	m.filtered = m.applyFilter()
	m.cursor = min(m.cursor, max(0, len(m.filtered)-1))
	m.syncViewport()
	return m, nil
}

// setSize propagates terminal dimensions to the sub-model and resizes the
// viewport. Called by the root model on every tea.WindowSizeMsg.
func (m *listModel) setSize(width, height int) {
	m.width = width
	m.height = height

	leftW, rightW, paneH := m.paneWidths()
	m.searchInput.Width = leftW - 2
	m.vp.Width = rightW - 2
	m.vp.Height = paneH - 2

	m.syncViewport()
}

// Update handles messages when the list view is active.
// It returns (listModel, tea.Cmd) rather than (tea.Model, tea.Cmd) so the root
// model can update its stored listModel field directly.
func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	// Search mode: route input to the text field.
	if m.searching {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc", "enter":
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchQuery = m.searchInput.Value()
		m.filtered = m.applyFilter()
		m.cursor = min(m.cursor, max(0, len(m.filtered)-1))
		m.syncViewport()
		return m, cmd
	}

	// Confirm mode: wait for y/n.
	if m.confirm != confirmNone {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "y":
				fn := m.pendingFn
				m.confirm = confirmNone
				m.pendingFn = nil
				if fn != nil {
					if err := fn(); err != nil {
						return m, func() tea.Msg { return errMsg{err} }
					}
				}
				return m, func() tea.Msg { return reloadMsg{} }
			case "n", "esc":
				m.confirm = confirmNone
				m.pendingFn = nil
			}
		}
		return m, nil
	}

	// Normal mode.
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(km, keys.Search):
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case m.activePane == paneList:
		return m.updateListPane(km)

	case m.activePane == paneDetail:
		return m.updateDetailPane(km)
	}

	return m, nil
}

// updateListPane handles key events when the list pane is focused.
func (m listModel) updateListPane(km tea.KeyMsg) (listModel, tea.Cmd) {
	switch {
	case key.Matches(km, keys.Down):
		if len(m.filtered) > 0 {
			m.cursor = min(m.cursor+1, len(m.filtered)-1)
			m.syncViewport()
		}

	case key.Matches(km, keys.Up):
		m.cursor = max(m.cursor-1, 0)
		m.syncViewport()

	case key.Matches(km, keys.Right):
		m.activePane = paneDetail

	case key.Matches(km, keys.Create):
		return m, openEditor(createTemplate(), modeCreate, "")

	case key.Matches(km, keys.Edit):
		if t := m.selected(); t != nil {
			return m, openEditor(taskToTemplate(t), modeEdit, t.ID)
		}

	case key.Matches(km, keys.Promote):
		if t := m.selected(); t != nil {
			return m.doPromote(t)
		}

	case key.Matches(km, keys.Archive):
		if t := m.selected(); t != nil {
			if t.State == models.StateArchive {
				// restore is non-destructive — no confirm needed
				id := t.ID
				if _, err := tasks.Restore(m.boardDir, id); err != nil {
					return m, func() tea.Msg { return errMsg{err} }
				}
				return m, func() tea.Msg { return reloadMsg{} }
			}
			id := t.ID
			m.confirm = confirmArchive
			m.pendingFn = func() error {
				_, err := tasks.Archive(m.boardDir, id)
				return err
			}
		}

	case key.Matches(km, keys.Delete):
		if t := m.selected(); t != nil {
			id := t.ID
			m.confirm = confirmDelete
			m.pendingFn = func() error {
				return tasks.Delete(m.boardDir, id)
			}
		}
	}

	return m, nil
}

// updateDetailPane handles key events when the detail pane is focused.
// Scroll keys apply to the viewport; all action keys delegate to updateListPane
// so they work regardless of which pane is active.
func (m listModel) updateDetailPane(km tea.KeyMsg) (listModel, tea.Cmd) {
	switch {
	case key.Matches(km, keys.Down):
		m.vp.LineDown(1)
		return m, nil
	case key.Matches(km, keys.Up):
		m.vp.LineUp(1)
		return m, nil
	case key.Matches(km, keys.Left):
		m.activePane = paneList
		return m, nil
	}
	// All other keys (create, edit, promote, archive, delete, search…) behave
	// identically whether the list or detail pane is focused.
	return m.updateListPane(km)
}

// doPromote advances or retreats the selected task through the state machine:
// draft → active (Promote), active → draft (Demote), archive → draft (Restore).
func (m listModel) doPromote(t *models.Task) (listModel, tea.Cmd) {
	var err error
	switch t.State {
	case models.StateDraft:
		_, err = tasks.Promote(m.boardDir, t.ID)
	case models.StateActive:
		_, err = tasks.Demote(m.boardDir, t.ID)
	case models.StateArchive:
		_, err = tasks.Restore(m.boardDir, t.ID)
	}
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}
	return m, func() tea.Msg { return reloadMsg{} }
}

// View renders the full list view: two side-by-side panes and a footer bar.
func (m listModel) View() string {
	if m.width == 0 {
		return ""
	}

	leftW, rightW, paneH := m.paneWidths()
	innerLeftW := leftW - 2
	innerRightW := rightW - 2
	innerH := paneH - 2

	// Left pane.
	leftBorder := stylePaneInactive
	if m.activePane == paneList {
		leftBorder = stylePaneActive
	}
	leftPane := leftBorder.
		Width(innerLeftW).
		Height(innerH).
		Render(m.renderList(innerLeftW, innerH))

	// Right pane.
	rightBorder := stylePaneInactive
	if m.activePane == paneDetail {
		rightBorder = stylePaneActive
	}
	rightPane := rightBorder.
		Width(innerRightW).
		Height(innerH).
		Render(m.vp.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderFooter())
}

// renderList builds the string content for the left pane.
func (m listModel) renderList(innerW, innerH int) string {
	var lines []string

	visH := innerH

	// Search bar occupies the first line when active.
	if m.searching {
		lines = append(lines, m.searchInput.View())
		visH--
	}

	// Confirm prompt reserves the last line when pending.
	if m.confirm != confirmNone {
		visH--
	}

	// Scroll offset: keep the cursor in view.
	offset := 0
	if visH > 0 && m.cursor >= visH {
		offset = m.cursor - visH + 1
	}

	count := 0
	for i := offset; i < len(m.filtered) && count < visH; i++ {
		t := m.filtered[i]

		prefix := "  "
		rowStyle := styleCardNormal
		if i == m.cursor {
			prefix = "► "
			rowStyle = styleCardSelected
		}

		stateStr := fmt.Sprintf("%-7s", stateLabel(t.State))
		statusStr := statusStyle(string(t.Status)).Render(fmt.Sprintf("%-11s", string(t.Status)))
		idStr := styleID.Render(fmt.Sprintf("%-10s", t.ID))
		meta := fmt.Sprintf("%s%s %s %s ", prefix, idStr, stateStr, statusStr)
		metaW := lipgloss.Width(meta)
		titleW := innerW - metaW
		title := t.Title
		if titleW < 4 {
			titleW = 4
		}
		if len(title) > titleW {
			title = title[:titleW-1] + "…"
		}
		row := meta + title

		lines = append(lines, rowStyle.Render(row))
		count++
	}

	// Pad remaining lines so the pane height stays stable.
	for count < visH {
		lines = append(lines, "")
		count++
	}

	if m.confirm != confirmNone {
		lines = append(lines, m.confirmLine())
	}

	return strings.Join(lines, "\n")
}

// confirmLine returns the confirmation prompt as a styled one-liner.
func (m listModel) confirmLine() string {
	t := m.selected()
	id := ""
	if t != nil {
		id = t.ID
	}
	var prompt string
	switch m.confirm {
	case confirmArchive:
		prompt = fmt.Sprintf("archive %s? [y/n]", id)
	case confirmDelete:
		prompt = fmt.Sprintf("delete %s? [y/n]", id)
	}
	return styleError.Render(prompt)
}

// renderDetail produces the detail pane content string for a task.
func (m listModel) renderDetail(t *models.Task) string {
	if t == nil {
		return "(no task selected)"
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "title:   %s\n", t.Title)
	fmt.Fprintf(&sb, "state:   %s\n", string(t.State))
	fmt.Fprintf(&sb, "status:  %s\n",
		statusStyle(string(t.Status)).Render(string(t.Status)))
	fmt.Fprintf(&sb, "labels:  %s\n", strings.Join(t.Labels, ", "))
	fmt.Fprintf(&sb, "agent:   %s\n", t.Agent)

	created := ""
	if t.CreatedAt != nil {
		created = t.CreatedAt.Format("2006-01-02")
	}
	fmt.Fprintf(&sb, "created: %s\n", created)

	updated := ""
	if t.UpdatedAt != nil {
		updated = t.UpdatedAt.Format("2006-01-02")
	}
	fmt.Fprintf(&sb, "updated: %s\n", updated)

	sb.WriteString("\n── body ──\n")
	if strings.TrimSpace(t.Body) == "" {
		sb.WriteString("(no body)\n")
	} else {
		sb.WriteString(renderMarkdown(t.Body, m.vp.Width))
	}

	return sb.String()
}

// renderFooter renders the one-line key-hint bar at the bottom of the view.
// Content is context-sensitive: search mode, confirm mode, or normal navigation.
func (m listModel) renderFooter() string {
	var hints string
	switch {
	case m.searching:
		hints = "type to filter  ↵ confirm  esc cancel"
	case m.confirm != confirmNone:
		hints = "y confirm  n / esc cancel"
	default:
		hints = "j/k navigate  ←/→ panes  c create  e edit  p promote  a archive/restore  d delete  / search  tab→board  ? help  q quit"
	}
	return styleFooter.Width(m.width).Render("  " + hints)
}

// applyFilter returns the subset of allTasks whose title contains the current
// search query (case-insensitive substring match). Returns allTasks unchanged
// when the query is empty.
func (m listModel) applyFilter() []*models.Task {
	if m.searchQuery == "" {
		return m.allTasks
	}
	q := strings.ToLower(m.searchQuery)
	out := make([]*models.Task, 0, len(m.allTasks))
	for _, t := range m.allTasks {
		if strings.Contains(strings.ToLower(t.Title), q) {
			out = append(out, t)
		}
	}
	return out
}

// sortDescByID sorts tasks by the numeric suffix of their ID in descending
// order (task-10 before task-9 before task-1), ignoring state.
func sortDescByID(ts []*models.Task) {
	sort.Slice(ts, func(i, j int) bool {
		return taskIDNum(ts[i].ID) > taskIDNum(ts[j].ID)
	})
}

func taskIDNum(id string) int {
	if idx := strings.LastIndex(id, "-"); idx >= 0 {
		n, _ := strconv.Atoi(id[idx+1:])
		return n
	}
	return 0
}

// selectByID moves the cursor to the task with the given ID and focuses the
// detail pane. Used when navigating from the board view via Enter.
func (m *listModel) selectByID(id string) {
	for i, t := range m.filtered {
		if t.ID == id {
			m.cursor = i
			m.activePane = paneDetail
			m.syncViewport()
			return
		}
	}
}

// selected returns the currently highlighted task, or nil when the list is
// empty or the cursor is out of range.
func (m listModel) selected() *models.Task {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return m.filtered[m.cursor]
}

// syncViewport refreshes the detail viewport to reflect the selected task.
// Both content and scroll position are reset.
func (m *listModel) syncViewport() {
	if t := m.selected(); t != nil {
		m.vp.SetContent(m.renderDetail(t))
	} else {
		m.vp.SetContent("(no tasks)")
	}
	m.vp.GotoTop()
}

// renderMarkdown renders body text as styled Markdown via Glamour. Falls back
// to plain text if the renderer cannot be initialised or rendering fails.
func renderMarkdown(body string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return out
}

// paneWidths returns the outer dimensions (borders included) of each pane and
// the shared pane height. Left pane is ~35 % of terminal width; right pane
// takes the remainder minus a one-character gap. The footer row is excluded
// from the pane height.
func (m listModel) paneWidths() (leftW, rightW, paneH int) {
	leftW = int(float64(m.width) * 0.35)
	rightW = m.width - leftW - 1
	paneH = m.height - 1
	return
}

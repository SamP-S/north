// list.go — list+detail sub-model for the North TUI.
//
// The list holds navigation state only; action keys (c/e/m/s/d), search, and
// every modal live in the root Model so both views behave identically.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

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
	boardDir   string
	allTasks   []*models.Task
	filtered   []*models.Task
	filter     string
	sortKey    tasks.SortKey
	sortDesc   bool
	warnings   int
	cursor     int
	activePane paneMode
	vp         viewport.Model
	width      int
	height     int

	// Markdown rendering: the glamour style is detected once (querying the
	// terminal is only safe before Bubble Tea owns it) and the renderer is
	// cached per word-wrap width — building one per keystroke stalls input.
	mdStyle string
	md      *glamour.TermRenderer
	mdWidth int
}

// newListModel constructs a zeroed listModel for the given board directory.
func newListModel(boardDir string) listModel {
	return listModel{
		boardDir: boardDir,
		vp:       viewport.New(0, 0),
		mdStyle:  detectMarkdownStyle(),
		sortKey:  tasks.SortID,
		sortDesc: true, // newest first
	}
}

// detectMarkdownStyle picks the glamour style for task bodies. It runs during
// model construction — before tea.Program.Run — because the light/dark check
// sends an OSC query to the terminal and must not race Bubble Tea's input
// reader (doing so blocks the UI and swallows keystrokes).
func detectMarkdownStyle() string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		return "notty"
	}
	if lipgloss.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

// Init returns a command that triggers the initial data load via reloadMsg so
// the root model's reloadMsg handler populates both sub-models together.
func (m listModel) Init() tea.Cmd {
	return func() tea.Msg { return reloadMsg{} }
}

// reload loads all tasks from disk (tolerantly), applies the current filter,
// clamps the cursor, and refreshes the detail viewport.
func (m listModel) reload() (listModel, tea.Cmd) {
	snap, err := tasks.Load(m.boardDir)
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}
	m.allTasks = snap.Filter(nil, "")
	m.warnings = len(snap.Warnings)
	m.refilter()
	return m, nil
}

// setFilter applies a search query to the list.
func (m *listModel) setFilter(q string) {
	m.filter = q
	m.refilter()
}

// setSort changes the row ordering.
func (m *listModel) setSort(key tasks.SortKey, desc bool) {
	m.sortKey, m.sortDesc = key, desc
	m.refilter()
}

// refilter recomputes the visible rows, clamps the cursor, and refreshes the
// detail viewport.
func (m *listModel) refilter() {
	tasks.Sort(m.allTasks, m.sortKey, m.sortDesc)
	m.filtered = m.applyFilter()
	m.cursor = min(m.cursor, max(0, len(m.filtered)-1))
	m.syncViewport()
}

// setSize propagates terminal dimensions to the sub-model and resizes the
// viewport. Called by the root model on every tea.WindowSizeMsg.
func (m *listModel) setSize(width, height int) {
	m.width = width
	m.height = height

	_, rightW, paneH := m.paneWidths()
	m.vp.Width = rightW - 2
	m.vp.Height = paneH - 2

	m.syncViewport()
}

// Update handles messages when the list view is active.
func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.activePane == paneDetail {
		return m.updateDetailPane(km)
	}
	return m.updateListPane(km)
}

// updateListPane handles navigation when the list pane is focused.
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

	case key.Matches(km, keys.Top):
		m.cursor = 0
		m.syncViewport()

	case key.Matches(km, keys.Bottom):
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
			m.syncViewport()
		}

	case key.Matches(km, keys.Right), key.Matches(km, keys.Enter):
		m.activePane = paneDetail
	}

	return m, nil
}

// updateDetailPane handles key events when the detail pane is focused.
func (m listModel) updateDetailPane(km tea.KeyMsg) (listModel, tea.Cmd) {
	switch {
	case key.Matches(km, keys.Down):
		m.vp.LineDown(1)
	case key.Matches(km, keys.Up):
		m.vp.LineUp(1)
	case key.Matches(km, keys.Top):
		m.vp.GotoTop()
	case key.Matches(km, keys.Bottom):
		m.vp.GotoBottom()
	case key.Matches(km, keys.Left), key.Matches(km, keys.Esc):
		m.activePane = paneList
	}
	return m, nil
}

// View renders the two side-by-side panes (the root owns status bar + footer).
func (m listModel) View() string {
	if m.width == 0 {
		return ""
	}

	leftW, rightW, paneH := m.paneWidths()
	innerLeftW := leftW - 2
	innerRightW := rightW - 2
	innerH := paneH - 2

	// Left pane.
	leftBorder := th.PaneInactive
	if m.activePane == paneList {
		leftBorder = th.PaneActive
	}
	leftPane := leftBorder.
		Width(innerLeftW).
		Height(innerH).
		Render(m.renderList(innerLeftW, innerH))

	// Right pane.
	rightBorder := th.PaneInactive
	if m.activePane == paneDetail {
		rightBorder = th.PaneActive
	}
	rightPane := rightBorder.
		Width(innerRightW).
		Height(innerH).
		Render(m.vp.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
}

// renderList builds the string content for the left pane.
func (m listModel) renderList(innerW, innerH int) string {
	var lines []string

	visH := innerH

	// Scroll offset: keep the cursor in view.
	offset := 0
	if visH > 0 && m.cursor >= visH {
		offset = m.cursor - visH + 1
	}

	count := 0
	for i := offset; i < len(m.filtered) && count < visH; i++ {
		t := m.filtered[i]

		selected := i == m.cursor
		prefix := "  "
		if selected {
			prefix = "► "
		}

		// Each segment styles (and bolds) itself: a styled segment ends with
		// an ANSI reset that would cancel one outer row-wide bold.
		stateStr := stateStyle(t.State).Bold(selected).Render(fmt.Sprintf("%-7s", stateLabel(t.State)))
		statusStr := statusStyle(string(t.Status)).Bold(selected).Render(fmt.Sprintf("%-11s", string(t.Status)))
		idStr := th.ID.Bold(selected).Render(fmt.Sprintf("%-4s", t.ID))
		meta := fmt.Sprintf("%s%s %s %s ", prefix, idStr, stateStr, statusStr)
		metaW := lipgloss.Width(meta)
		titleW := innerW - metaW
		if titleW < 4 {
			titleW = 4
		}
		// Truncate by display width so multibyte/wide titles never break.
		title := ansi.Truncate(t.Title, titleW, "…")
		row := meta + title
		if selected {
			row = th.CardSelected.Render(prefix) + idStr + " " + stateStr +
				" " + statusStr + th.CardSelected.Render(" "+title)
		}

		lines = append(lines, row)
		count++
	}

	if len(m.filtered) == 0 {
		hint := "no tasks — press c to create"
		if m.filter != "" {
			hint = "no tasks match the filter — esc clears it"
		}
		lines = append(lines, th.ID.Render(hint))
		count++
	}

	// Pad remaining lines so the pane height stays stable.
	for count < visH {
		lines = append(lines, "")
		count++
	}

	return strings.Join(lines, "\n")
}

// renderDetail produces the detail pane content string for a task.
func (m listModel) renderDetail(t *models.Task) string {
	if t == nil {
		return "(no task selected)"
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "id:          %s\n", th.ID.Render(t.ID))
	fmt.Fprintf(&sb, "title:       %s\n", t.Title)
	fmt.Fprintf(&sb, "state:       %s\n", stateStyle(t.State).Render(string(t.State)))
	fmt.Fprintf(&sb, "status:      %s\n",
		statusStyle(string(t.Status)).Render(string(t.Status)))
	fmt.Fprintf(&sb, "labels:      %s\n", strings.Join(t.Labels, ", "))
	fmt.Fprintf(&sb, "assignee:    %s\n", t.Assignee)
	fmt.Fprintf(&sb, "depends_on:  %s\n", m.renderDeps(t))

	const tsFormat = "2006-01-02 15:04:05" // timestamps are stored (and shown) in UTC
	created := ""
	if t.CreatedAt != nil {
		created = t.CreatedAt.Format(tsFormat)
	}
	fmt.Fprintf(&sb, "created_at:  %s\n", created)

	updated := ""
	if t.UpdatedAt != nil {
		updated = t.UpdatedAt.Format(tsFormat)
	}
	fmt.Fprintf(&sb, "updated_at:  %s\n", updated)

	sb.WriteString("\n── body ──\n")
	if strings.TrimSpace(t.Body) == "" {
		sb.WriteString("(no body)\n")
	} else {
		sb.WriteString(m.renderMarkdown(t.Body))
	}

	return sb.String()
}

// renderDeps renders a task's depends_on entries as "id title (status:state)"
// — the deps picker's format — one per line, aligned under the field label.
func (m listModel) renderDeps(t *models.Task) string {
	if len(t.DependsOn) == 0 {
		return ""
	}
	byID := map[string]*models.Task{}
	for _, other := range m.allTasks {
		byID[other.ID] = other
	}
	parts := make([]string, len(t.DependsOn))
	for i, dep := range t.DependsOn {
		if d, ok := byID[dep]; ok {
			parts[i] = fmt.Sprintf("%s %s (%s:%s)", dep, d.Title, d.Status, d.State)
		} else {
			parts[i] = fmt.Sprintf("%s (missing)", dep)
		}
	}
	return strings.Join(parts, "\n             ") // align under "depends_on:  "
}

// applyFilter returns the subset of allTasks whose id, title, or labels
// contain the current filter (case-insensitive substring match).
func (m listModel) applyFilter() []*models.Task {
	if m.filter == "" {
		return m.allTasks
	}
	out := make([]*models.Task, 0, len(m.allTasks))
	for _, t := range m.allTasks {
		if matchesFilter(t, m.filter) {
			out = append(out, t)
		}
	}
	return out
}

// matchesFilter reports whether a task's id, title, assignee, labels, or body
// contain the query (case-insensitive substring match, same fields as the
// CLI's list --search). Shared by the board and list.
func matchesFilter(t *models.Task, query string) bool {
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(t.Title), q) ||
		strings.Contains(strings.ToLower(t.ID), q) ||
		strings.Contains(strings.ToLower(t.Assignee), q) ||
		strings.Contains(strings.ToLower(strings.Join(t.Labels, "\n")), q) ||
		strings.Contains(strings.ToLower(t.Body), q)
}

// selectByID moves the cursor to the task with the given ID and focuses the
// detail pane. Returns false when the task is not in the visible rows (e.g.
// hidden by the active filter) — the root clears the filter and retries.
func (m *listModel) selectByID(id string) bool {
	for i, t := range m.filtered {
		if t.ID == id {
			m.cursor = i
			m.activePane = paneDetail
			m.syncViewport()
			return true
		}
	}
	return false
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
	m.ensureRenderer()
	if t := m.selected(); t != nil {
		m.vp.SetContent(m.renderDetail(t))
	} else {
		m.vp.SetContent("(no tasks)")
	}
	m.vp.GotoTop()
}

// ensureRenderer (re)builds the cached glamour renderer when the detail-pane
// width changes. WithStandardStyle deliberately avoids WithAutoStyle, which
// queries the terminal and would fight Bubble Tea for input.
func (m *listModel) ensureRenderer() {
	if m.md != nil && m.mdWidth == m.vp.Width {
		return
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.mdStyle),
		glamour.WithWordWrap(m.vp.Width),
	)
	if err != nil {
		return // keep the previous renderer (or none: plain-text fallback)
	}
	m.md = r
	m.mdWidth = m.vp.Width
}

// renderMarkdown renders body text as styled Markdown via the cached Glamour
// renderer. Falls back to plain text if none is available or rendering fails.
func (m listModel) renderMarkdown(body string) string {
	if m.md == nil {
		return body
	}
	out, err := m.md.Render(body)
	if err != nil {
		return body
	}
	return out
}

// paneWidths returns the outer dimensions (borders included) of each pane and
// the shared pane height. Left pane is ~35 % of terminal width; right pane
// takes the remainder minus a one-character gap.
func (m listModel) paneWidths() (leftW, rightW, paneH int) {
	leftW = int(float64(m.width) * 0.35)
	rightW = m.width - leftW - 1
	paneH = m.height
	return
}

// list.go — list+detail sub-model for the North TUI.
//
// The list holds navigation and search state only; action keys (c/e/m/s/d)
// and every modal live in the root Model so both views behave identically.
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
	boardDir    string
	allTasks    []*models.Task
	filtered    []*models.Task
	warnings    int
	cursor      int
	activePane  paneMode
	vp          viewport.Model
	searchInput textinput.Model
	searching   bool
	searchQuery string
	width       int
	height      int

	// Markdown rendering: the glamour style is detected once (querying the
	// terminal is only safe before Bubble Tea owns it) and the renderer is
	// cached per word-wrap width — building one per keystroke stalls input.
	mdStyle string
	md      *glamour.TermRenderer
	mdWidth int
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
		mdStyle:     detectMarkdownStyle(),
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
	ts := snap.Filter(nil, "")
	sortDescByID(ts)
	m.allTasks = ts
	m.warnings = len(snap.Warnings)
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
	case key.Matches(km, keys.Left), key.Matches(km, keys.Esc):
		m.activePane = paneList
	}
	return m, nil
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
		idStr := styleID.Render(fmt.Sprintf("%-4s", t.ID))
		meta := fmt.Sprintf("%s%s %s %s ", prefix, idStr, stateStr, statusStr)
		metaW := lipgloss.Width(meta)
		titleW := innerW - metaW
		if titleW < 4 {
			titleW = 4
		}
		// Truncate by display width so multibyte/wide titles never break.
		title := ansi.Truncate(t.Title, titleW, "…")
		row := meta + title

		lines = append(lines, rowStyle.Render(row))
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

	fmt.Fprintf(&sb, "id:          %s\n", styleID.Render(t.ID))
	fmt.Fprintf(&sb, "title:       %s\n", t.Title)
	fmt.Fprintf(&sb, "state:       %s\n", string(t.State))
	fmt.Fprintf(&sb, "status:      %s\n",
		statusStyle(string(t.Status)).Render(string(t.Status)))
	fmt.Fprintf(&sb, "labels:      %s\n", strings.Join(t.Labels, ", "))
	fmt.Fprintf(&sb, "agent:       %s\n", t.Agent)
	fmt.Fprintf(&sb, "depends_on:  %s\n", m.renderDeps(t))

	created := ""
	if t.CreatedAt != nil {
		created = t.CreatedAt.Format("2006-01-02")
	}
	fmt.Fprintf(&sb, "created_at:  %s\n", created)

	updated := ""
	if t.UpdatedAt != nil {
		updated = t.UpdatedAt.Format("2006-01-02")
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

// renderDeps renders a task's depends_on ids with each dependency's current
// status, e.g. "4 (done), 7 (ready)".
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
			parts[i] = fmt.Sprintf("%s (%s)", dep, d.Status)
		} else {
			parts[i] = fmt.Sprintf("%s (missing)", dep)
		}
	}
	return strings.Join(parts, ", ")
}

// renderFooter renders the one-line key-hint bar at the bottom of the view.
func (m listModel) renderFooter() string {
	var hints string
	if m.searching {
		hints = "type to filter  ↵ confirm  esc cancel"
	} else {
		hints = "j/k navigate  ←/→ panes  c create  e edit  m status  s state  d delete  r reload  / search  tab→board  ? help  q quit"
	}
	if m.warnings > 0 {
		hints = fmt.Sprintf("⚠ %d file warning(s)  ", m.warnings) + hints
	}
	return styleFooter.Width(m.width).Render("  " + hints)
}

// applyFilter returns the subset of allTasks whose id, title, or labels
// contain the current search query (case-insensitive substring match).
func (m listModel) applyFilter() []*models.Task {
	if m.searchQuery == "" {
		return m.allTasks
	}
	q := strings.ToLower(m.searchQuery)
	out := make([]*models.Task, 0, len(m.allTasks))
	for _, t := range m.allTasks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.ID), q) ||
			strings.Contains(strings.ToLower(strings.Join(t.Labels, "\n")), q) {
			out = append(out, t)
		}
	}
	return out
}

// sortDescByID sorts tasks by their numeric id in descending order (newest
// first), ignoring state.
func sortDescByID(ts []*models.Task) {
	sort.Slice(ts, func(i, j int) bool {
		return taskIDNum(ts[i].ID) > taskIDNum(ts[j].ID)
	})
}

func taskIDNum(id string) int {
	n, _ := strconv.Atoi(id)
	return n
}

// selectByID moves the cursor to the task with the given ID and focuses the
// detail pane. When the task is hidden by an active filter, the filter is
// cleared first so the selection can never silently fail.
func (m *listModel) selectByID(id string) {
	find := func() int {
		for i, t := range m.filtered {
			if t.ID == id {
				return i
			}
		}
		return -1
	}
	i := find()
	if i < 0 && m.searchQuery != "" {
		m.searchQuery = ""
		m.searchInput.SetValue("")
		m.filtered = m.applyFilter()
		i = find()
	}
	if i < 0 {
		return
	}
	m.cursor = i
	m.activePane = paneDetail
	m.syncViewport()
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
// takes the remainder minus a one-character gap. The footer row is excluded
// from the pane height.
func (m listModel) paneWidths() (leftW, rightW, paneH int) {
	leftW = int(float64(m.width) * 0.35)
	rightW = m.width - leftW - 1
	paneH = m.height - 1
	return
}

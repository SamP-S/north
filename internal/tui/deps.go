// deps.go — the w link modal: a multi-select picker over the board's tasks
// that edits the selected task's depends_on.
//
// The list shows every draft and active task plus the task's existing
// dependencies whatever their state, so applying can never silently drop a
// dep. Resolved candidates (done or archived) are selectable but marked ✓ —
// such a link is born satisfied and documents lineage rather than gating.
// Invalid choices (the task itself, anything that would create a cycle) are
// greyed with a reason instead of hidden; the picker never assembles an
// illegal set, at any deps_enforcement level. `/` filters in place with the
// same matcher as the main views.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// depEntry is one pickable row. task is nil for a dependency id that no
// longer resolves to a task (a dangling ref on a hint-level board) — shown
// so applying preserves rather than silently drops it.
type depEntry struct {
	id       string
	task     *models.Task
	invalid  string // "" selectable, else the reason ("self", "cycle")
	resolved bool   // done or archived — the link would be born satisfied
}

// newDepsModal builds the w modal for task t. Entries follow the root sort
// within two groups: draft+active first, archive last; dangling existing
// deps lead the list so they are impossible to miss.
func newDepsModal(boardDir string, t *models.Task, key tasks.SortKey, desc bool, width, height int) (modal, error) {
	snap, err := tasks.Load(boardDir)
	if err != nil {
		return modal{}, err
	}
	blockers := snap.TransitiveDependents(t.ID)

	live := snap.Filter([]models.TaskState{models.StateDraft, models.StateActive}, "")
	arch := snap.Filter([]models.TaskState{models.StateArchive}, "")
	tasks.Sort(live, key, desc)
	tasks.Sort(arch, key, desc)

	var entries []depEntry
	for _, dep := range t.DependsOn {
		if snap.Get(dep) == nil {
			entries = append(entries, depEntry{id: dep})
		}
	}
	for _, group := range [][]*models.Task{live, arch} {
		for _, c := range group {
			e := depEntry{
				id:       c.ID,
				task:     c,
				resolved: c.Status == models.Done || c.State == models.StateArchive,
			}
			switch {
			case c.ID == t.ID:
				e.invalid = "self"
			case blockers[c.ID]:
				e.invalid = "cycle"
			}
			entries = append(entries, e)
		}
	}

	checked := make(map[string]bool, len(t.DependsOn))
	for _, dep := range t.DependsOn {
		checked[dep] = true
	}

	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.CharLimit = 120
	fi.Prompt = "/ "

	return modal{
		mode:        modalDepsPicker,
		taskID:      t.ID,
		heading:     fmt.Sprintf("%s %s (%s:%s)", t.ID, t.Title, t.Status, t.State),
		entries:     entries,
		checked:     checked,
		filterInput: fi,
		rows:        max(4, height-12),
		width:       min(96, max(44, width*3/4)),
	}, nil
}

// matchEntry reports whether an entry matches the filter query, using the
// same fields as the main views' live filter.
func matchEntry(e depEntry, q string) bool {
	if q == "" {
		return true
	}
	if e.task == nil {
		return strings.Contains(strings.ToLower(e.id), q)
	}
	t := e.task
	return strings.Contains(strings.ToLower(t.ID), q) ||
		strings.Contains(strings.ToLower(t.Title), q) ||
		strings.Contains(strings.ToLower(t.Assignee), q) ||
		strings.Contains(strings.ToLower(t.Body), q) ||
		strings.Contains(strings.ToLower(strings.Join(t.Labels, "\n")), q)
}

// visibleEntries returns the indices of entries passing the current filter.
// Index -1 stands for the pinned "clear all" row, always first.
func (m modal) visibleEntries() []int {
	q := strings.ToLower(m.filterInput.Value())
	out := []int{-1}
	for i, e := range m.entries {
		if matchEntry(e, q) {
			out = append(out, i)
		}
	}
	return out
}

// updateDeps handles a key while the w modal is open.
func (m modal) updateDeps(km tea.KeyMsg, boardDir string) (modal, tea.Cmd) {
	// The filter input captures every key except esc/enter.
	if m.filterOn {
		switch km.String() {
		case "esc", "enter":
			m.filterOn = false
			m.filterInput.Blur()
			return m, nil
		}
		m.filterInput, _ = m.filterInput.Update(km)
		m.cursor = 0
		return m, nil
	}

	vis := m.visibleEntries()
	if m.cursor >= len(vis) {
		m.cursor = len(vis) - 1
	}
	switch km.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(vis)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(vis) - 1
	case "/":
		m.filterOn = true
		m.filterInput.Focus()
	case " ":
		idx := vis[m.cursor]
		if idx == -1 {
			m.checked = map[string]bool{}
			m.note = "cleared — enter applies"
			break
		}
		e := m.entries[idx]
		if e.invalid != "" {
			if e.invalid == "self" {
				m.note = "a task cannot depend on itself"
			} else {
				m.note = fmt.Sprintf("linking %s would create a cycle", e.id)
			}
			break
		}
		m.checked[e.id] = !m.checked[e.id]
	case "enter":
		deps := make([]string, 0, len(m.checked))
		for _, e := range m.entries {
			if m.checked[e.id] {
				deps = append(deps, e.id)
			}
		}
		taskID := m.taskID
		text := fmt.Sprintf("%s depends on %s", taskID, strings.Join(deps, ", "))
		if len(deps) == 0 {
			text = "cleared dependencies of " + taskID
		}
		m.mode = modalNone
		return m, runTaskOp(func() ([]string, error) {
			_, warns, err := tasks.Edit(boardDir, taskID, tasks.EditOpts{DependsOn: &deps})
			return warns, err
		}, notice{noticeSuccess, text})
	case "esc":
		if m.filterInput.Value() != "" {
			m.filterInput.SetValue("")
			m.cursor = 0
			break
		}
		m.mode = modalNone
	}
	return m, nil
}

// depsView renders the w modal, sized to the stored width (¾ of the
// terminal, clamped) with over-long lines truncated rather than wrapped.
func (m modal) depsView() string {
	textW := m.width - 2 // border padding
	var sb strings.Builder
	sb.WriteString(th.Header.Render(ansi.Truncate("depends on — "+m.heading, textW, "…")) + "\n")
	if m.filterOn || m.filterInput.Value() != "" {
		sb.WriteString(m.filterInput.View() + "\n")
	}
	sb.WriteString("\n")

	vis := m.visibleEntries()
	cursor := m.cursor
	if cursor >= len(vis) {
		cursor = len(vis) - 1
	}
	// Scroll window: keep the cursor visible within the row budget.
	start := 0
	if cursor >= m.rows {
		start = cursor - m.rows + 1
	}
	end := min(len(vis), start+m.rows)

	for i := start; i < end; i++ {
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		idx := vis[i]
		if idx == -1 {
			sb.WriteString(prefix + "(clear all)\n")
			continue
		}
		e := m.entries[idx]
		box := "[ ]"
		if m.checked[e.id] {
			box = "[x]"
		}
		line := ansi.Truncate(fmt.Sprintf("%s%s %s", prefix, box, depsLabel(e)), textW, "…")
		switch {
		case e.invalid != "":
			line = th.Footer.Render(line)
		case e.task != nil && e.task.State == models.StateArchive:
			line = th.Footer.Render(line)
		}
		sb.WriteString(line + "\n")
	}
	if len(vis) > end {
		sb.WriteString(th.Footer.Render(fmt.Sprintf("  … %d more", len(vis)-end)) + "\n")
	}

	if m.note != "" {
		sb.WriteString(noticeStyle(noticeWarn).Render(m.note) + "\n")
	}
	sb.WriteString(th.Footer.Render("space toggle  / filter  enter apply  esc close"))
	return th.Modal.Width(m.width).Render(sb.String())
}

// depsLabel renders one entry: `✓ 12 Add login (done:active)`, with a
// trailing reason when the entry is invalid and `(missing)` for dangling ids.
func depsLabel(e depEntry) string {
	mark := "  "
	if e.resolved {
		mark = "✓ "
	}
	if e.task == nil {
		return mark + e.id + " (missing)"
	}
	label := fmt.Sprintf("%s%s %s (%s:%s)", mark, e.task.ID, e.task.Title, e.task.Status, e.task.State)
	if e.invalid != "" {
		label += " — " + e.invalid
	}
	return label
}

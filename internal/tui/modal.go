// modal.go — the single modal layer shared by both views.
//
// All modals (status picker, state picker, delete confirm) are owned by the
// root Model, so the board and list views behave identically for the same key
// and hold no modal state of their own.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// modalMode identifies the active overlay modal.
type modalMode int

const (
	modalNone          modalMode = iota
	modalStatusPicker            // set a task's status (m)
	modalStatePicker             // set any task's state (s)
	modalConfirmDelete           // confirm a delete (d)
	modalTaskView                // read-only task popup (enter, board view)
	modalSortPicker              // choose the task ordering (o)
	modalDoctor                  // board integrity report (x; f applies --fix)
	modalDepsPicker              // edit a task's depends_on (w)
)

// sortMsg carries a chosen ordering from the sort picker to the root model.
type sortMsg struct {
	key  tasks.SortKey
	desc bool
}

// sortChoices lists the picker entries: each key descending, then ascending.
func sortChoices() []sortMsg {
	var out []sortMsg
	for _, k := range tasks.SortKeys {
		out = append(out, sortMsg{k, true}, sortMsg{k, false})
	}
	return out
}

// sortLabel renders a picker entry ("updated ↓").
func sortLabel(c sortMsg) string {
	arrow := "↑"
	if c.desc {
		arrow = "↓"
	}
	return string(c.key) + " " + arrow
}

// sortIndex returns the picker index for the given ordering.
func sortIndex(key tasks.SortKey, desc bool) int {
	for i, c := range sortChoices() {
		if c.key == key && c.desc == desc {
			return i
		}
	}
	return 0
}

// modal is the root model's modal state.
type modal struct {
	mode      modalMode
	cursor    int    // picker cursor
	taskID    string // task the modal acts on
	taskState models.TaskState
	confirm   string         // prompt text for the delete confirm
	vp        viewport.Model // scrollable content for the task popup
	note      string         // transient in-modal feedback, cleared on the next key

	// w deps-picker state.
	heading     string // edited task's "id title (status:state)" line
	entries     []depEntry
	checked     map[string]bool
	filterInput textinput.Model
	filterOn    bool // the filter input has focus (keys type, not toggle)
	rows        int  // list-window row budget
	width       int  // modal block width
}

func (m modal) open() bool { return m.mode != modalNone }

// pickerItems returns the item list for the active picker.
func (m modal) pickerItems() []string {
	switch m.mode {
	case modalStatusPicker:
		items := make([]string, len(models.Statuses))
		for i, s := range models.Statuses {
			items[i] = string(s)
		}
		return items
	case modalStatePicker:
		items := make([]string, len(models.StateOrder))
		for i, s := range models.StateOrder {
			items[i] = string(s)
		}
		return items
	case modalSortPicker:
		choices := sortChoices()
		items := make([]string, len(choices))
		for i, c := range choices {
			items[i] = sortLabel(c)
		}
		return items
	}
	return nil
}

// update handles a key while a modal is open. It returns the new modal state
// and a command to run when the modal resolved into an action.
func (m modal) update(km tea.KeyMsg, boardDir string) (modal, tea.Cmd) {
	m.note = ""
	switch m.mode {
	case modalDepsPicker:
		return m.updateDeps(km, boardDir)
	case modalSortPicker:
		items := m.pickerItems()
		switch {
		case key.Matches(km, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(km, keys.Down):
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case key.Matches(km, keys.Enter):
			choice := sortChoices()[m.cursor]
			m.mode = modalNone
			return m, func() tea.Msg { return choice }
		case key.Matches(km, keys.Esc):
			m.mode = modalNone
		}
		return m, nil

	case modalStatusPicker, modalStatePicker:
		items := m.pickerItems()
		switch {
		case key.Matches(km, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(km, keys.Down):
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case key.Matches(km, keys.Enter):
			choice := items[m.cursor]
			taskID := m.taskID
			mode := m.mode
			ok := notice{noticeSuccess, fmt.Sprintf("%s state → %s", taskID, choice)}
			if mode == modalStatusPicker {
				ok = notice{noticeSuccess, fmt.Sprintf("%s → %s", taskID, choice)}
				if m.taskState != models.StateActive {
					ok = notice{noticeWarn, fmt.Sprintf(
						"%s → %s (%s — shows on the board once active)",
						taskID, choice, m.taskState)}
				}
			}
			m.mode = modalNone
			return m, runTaskOp(func() ([]string, error) {
				if mode == modalStatusPicker {
					_, warns, err := tasks.SetStatus(boardDir, taskID, choice)
					return warns, err
				}
				_, err := tasks.SetState(boardDir, taskID, choice)
				return nil, err
			}, ok)
		case key.Matches(km, keys.Esc):
			m.mode = modalNone
		}
		return m, nil

	case modalTaskView:
		switch {
		case key.Matches(km, keys.Up):
			m.vp.LineUp(1)
		case key.Matches(km, keys.Down):
			m.vp.LineDown(1)
		case key.Matches(km, keys.Top):
			m.vp.GotoTop()
		case key.Matches(km, keys.Bottom):
			m.vp.GotoBottom()
		case key.Matches(km, keys.Esc), key.Matches(km, keys.Enter):
			m.mode = modalNone
		case key.Matches(km, keys.Edit):
			// Close the popup and drop straight into the editor.
			taskID := m.taskID
			m.mode = modalNone
			t, err := tasks.Get(boardDir, taskID)
			if err != nil {
				return m, func() tea.Msg { return errMsg{err} }
			}
			tmpl, err := editTemplate(t)
			if err != nil {
				return m, func() tea.Msg { return errMsg{err} }
			}
			return m, openEditor(tmpl, modeEdit, taskID)
		}
		return m, nil

	case modalDoctor:
		switch {
		case key.Matches(km, keys.Up):
			m.vp.LineUp(1)
		case key.Matches(km, keys.Down):
			m.vp.LineDown(1)
		case key.Matches(km, keys.Top):
			m.vp.GotoTop()
		case key.Matches(km, keys.Bottom):
			m.vp.GotoBottom()
		case km.String() == "f":
			issues, err := tasks.Doctor(boardDir, true)
			if err != nil {
				m.mode = modalNone
				return m, func() tea.Msg { return errMsg{err} }
			}
			m.vp.SetContent(doctorContent(issues))
			m.vp.GotoTop()
			// The board reloads underneath while the fix report stays open.
			return m, func() tea.Msg { return reloadMsg{} }
		case key.Matches(km, keys.Esc), key.Matches(km, keys.Enter):
			m.mode = modalNone
		}
		return m, nil

	case modalConfirmDelete:
		switch strings.ToLower(km.String()) {
		case "y", "enter":
			taskID := m.taskID
			m.mode = modalNone
			return m, runTaskOp(
				func() ([]string, error) { return tasks.Delete(boardDir, taskID) },
				notice{noticeSuccess, "deleted " + taskID})
		case "n", "esc":
			m.mode = modalNone
		}
		return m, nil
	}
	return m, nil
}

// view renders the active modal's content (to be centered by the caller).
func (m modal) view() string {
	switch m.mode {
	case modalStatusPicker, modalStatePicker, modalSortPicker:
		title := "set status"
		switch m.mode {
		case modalStatePicker:
			title = "set state"
		case modalSortPicker:
			title = "sort by"
		}
		var sb strings.Builder
		sb.WriteString(th.Header.Render(title) + "\n\n")
		for i, item := range m.pickerItems() {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			// Colors only: status entries take the status color, state
			// entries the state color; sort entries stay plain.
			style := lipgloss.NewStyle()
			switch m.mode {
			case modalStatusPicker:
				style = statusStyle(item)
			case modalStatePicker:
				style = stateStyle(models.TaskState(item))
			}
			sb.WriteString(prefix + style.Render(item) + "\n")
		}
		return th.Modal.Render(strings.TrimRight(sb.String(), "\n"))
	case modalConfirmDelete:
		return th.Modal.Render(m.confirm)
	case modalTaskView:
		footer := th.Footer.Render("j/k scroll  g/G top/bottom  e edit  y yank  esc close")
		if m.note != "" {
			footer = noticeStyle(noticeSuccess).Render(m.note) + "\n" + footer
		}
		return th.Modal.Render(m.vp.View() + "\n" + footer)
	case modalDoctor:
		footer := th.Footer.Render("j/k scroll  f fix  esc close")
		return th.Modal.Render(th.Header.Render("doctor") + "\n\n" + m.vp.View() + "\n" + footer)
	case modalDepsPicker:
		return m.depsView()
	}
	return ""
}

// newDoctorModal builds the doctor popup around a fresh report.
func newDoctorModal(issues []tasks.Issue, width, height int) modal {
	vp := viewport.New(max(20, width-14), max(5, height-8))
	vp.SetContent(doctorContent(issues))
	return modal{mode: modalDoctor, vp: vp}
}

// doctorContent renders a doctor report for the popup viewport.
func doctorContent(issues []tasks.Issue) string {
	if len(issues) == 0 {
		return "Board is healthy — no issues found."
	}
	lines := make([]string, len(issues))
	for i, is := range issues {
		lines[i] = is.String()
	}
	return strings.Join(lines, "\n")
}

// statusIndex returns the index of s in models.Statuses, defaulting to 0.
func statusIndex(s models.TaskStatus) int {
	for i, st := range models.Statuses {
		if st == s {
			return i
		}
	}
	return 0
}

// stateIndex returns the index of s in models.StateOrder, defaulting to 0.
func stateIndex(s models.TaskState) int {
	for i, st := range models.StateOrder {
		if st == s {
			return i
		}
	}
	return 0
}

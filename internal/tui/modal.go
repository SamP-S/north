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
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// modalMode identifies the active overlay modal.
type modalMode int

const (
	modalNone          modalMode = iota
	modalStatusPicker            // set an active task's status (m)
	modalStatePicker             // set any task's state (s)
	modalConfirmDelete           // confirm a delete (d)
)

// modal is the root model's modal state.
type modal struct {
	mode      modalMode
	cursor    int    // picker cursor
	taskID    string // task the modal acts on
	taskState models.TaskState
	confirm   string // prompt text for the delete confirm
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
	}
	return nil
}

// update handles a key while a modal is open. It returns the new modal state
// and a command to run when the modal resolved into an action.
func (m modal) update(km tea.KeyMsg, boardDir string) (modal, tea.Cmd) {
	switch m.mode {
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
			return m, runTaskOp(func() error {
				var err error
				if mode == modalStatusPicker {
					_, err = tasks.SetStatus(boardDir, taskID, choice)
				} else {
					_, err = tasks.SetState(boardDir, taskID, choice)
				}
				return err
			}, ok)
		case key.Matches(km, keys.Esc):
			m.mode = modalNone
		}
		return m, nil

	case modalConfirmDelete:
		switch strings.ToLower(km.String()) {
		case "y", "enter":
			taskID := m.taskID
			m.mode = modalNone
			return m, runTaskOp(
				func() error { return tasks.Delete(boardDir, taskID) },
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
	case modalStatusPicker, modalStatePicker:
		title := "set status"
		if m.mode == modalStatePicker {
			title = "set state"
		}
		var sb strings.Builder
		sb.WriteString(styleHeader.Render(title) + "\n\n")
		for i, item := range m.pickerItems() {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			sb.WriteString(prefix + statusStyle(item).Render(item) + "\n")
		}
		return styleModal.Render(strings.TrimRight(sb.String(), "\n"))
	case modalConfirmDelete:
		return styleModal.Render(m.confirm)
	}
	return ""
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

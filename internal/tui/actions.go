// actions.go — task operations and modal text shared between the board and
// list views, so the two views behave identically for the same key.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// promoteOrDemote returns a tea.Cmd that promotes a draft task onto the active
// board or demotes an active task back to draft. It is a no-op for archived
// tasks — restoring an archived task is the Archive key's job, not Promote's,
// so each key maps to exactly one kind of change.
func promoteOrDemote(boardDir string, t *models.Task) tea.Cmd {
	if t == nil {
		return nil
	}
	switch t.State {
	case models.StateDraft:
		return runTaskOp(func() error { _, err := tasks.Promote(boardDir, t.ID); return err })
	case models.StateActive:
		return runTaskOp(func() error { _, err := tasks.Demote(boardDir, t.ID); return err })
	default:
		return nil
	}
}

// runTaskOp wraps a task mutation into a tea.Cmd: errMsg on failure, reloadMsg
// on success.
func runTaskOp(op func() error) tea.Cmd {
	return func() tea.Msg {
		if err := op(); err != nil {
			return errMsg{err}
		}
		return reloadMsg{}
	}
}

// deleteConfirmText builds the delete-confirmation prompt for a task,
// including a dependents warning when other tasks reference it. Used by both
// the board and list views so the warning is not board-only.
func deleteConfirmText(boardDir string, t *models.Task) string {
	prompt := fmt.Sprintf("delete %s? [y/n]", t.ID)
	deps, err := tasks.Dependents(boardDir, t.ID)
	if err != nil || len(deps) == 0 {
		return prompt
	}
	ids := make([]string, len(deps))
	for i, d := range deps {
		ids[i] = d.ID
	}
	return fmt.Sprintf("delete %s?\n%s depend on this. [y/n]", t.ID, strings.Join(ids, ", "))
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

// renderStatusPicker renders the "move to status" modal body, highlighting
// cursor. Shared by the board and list views so the picker looks and behaves
// identically regardless of which view opened it.
func renderStatusPicker(cursor int) string {
	var sb strings.Builder
	sb.WriteString(styleHeader.Render("move to status") + "\n\n")
	for i, s := range models.Statuses {
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		sb.WriteString(prefix + statusStyle(string(s)).Render(string(s)) + "\n")
	}
	return styleModal.Render(strings.TrimRight(sb.String(), "\n"))
}

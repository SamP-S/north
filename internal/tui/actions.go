// actions.go — task operations shared between the board and list views.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// runTaskOp wraps a task mutation into a tea.Cmd: errMsg on failure, an
// actionDoneMsg carrying the given notice (which also triggers a reload) on
// success.
func runTaskOp(op func() error, ok notice) tea.Cmd {
	return func() tea.Msg {
		if err := op(); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{ok}
	}
}

// deleteConfirmText builds the delete-confirmation prompt for a task,
// including a dependents warning when other tasks reference it.
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

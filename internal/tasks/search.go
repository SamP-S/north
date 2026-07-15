package tasks

import (
	"strings"

	"github.com/SamP-S/north/internal/models"
)

// MatchesSearch reports whether a task's id, title, assignee, labels, or body
// contain the query (case-insensitive substring match; labels are matched
// individually). An empty query matches every task. This is the single
// matcher behind `task list --search` and the TUI's live and deps-picker
// filters.
func MatchesSearch(t *models.Task, query string) bool {
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(t.ID), q) ||
		strings.Contains(strings.ToLower(t.Title), q) ||
		strings.Contains(strings.ToLower(t.Assignee), q) ||
		strings.Contains(strings.ToLower(t.Body), q) ||
		strings.Contains(strings.ToLower(strings.Join(t.Labels, "\n")), q)
}

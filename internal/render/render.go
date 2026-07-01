// Package render produces CLI output: human, --plain, and --json.
//
// --plain is stable, tab/line-oriented text for scripts; --json is the
// structured Task map shape. The default is friendlier human output. North is
// agent-driven, so machine-readable output is first-class.
package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// TaskList renders a list of tasks.
func TaskList(tasks []*models.Task, plain, asJSON bool) (string, error) {
	if asJSON {
		summaries := make([]map[string]any, len(tasks))
		for i, t := range tasks {
			summaries[i] = summary(t)
		}
		return marshalJSON(summaries)
	}
	if plain {
		lines := make([]string, len(tasks))
		for i, t := range tasks {
			lines[i] = fmt.Sprintf("%s\t%s\t%s\t%s", t.ID, t.State, t.Status, t.Title)
		}
		return strings.Join(lines, "\n"), nil
	}
	if len(tasks) == 0 {
		return "(no tasks)", nil
	}
	width := 0
	for _, t := range tasks {
		if len(t.ID) > width {
			width = len(t.ID)
		}
	}
	var lines []string
	for _, t := range tasks {
		lines = append(lines, fmt.Sprintf("%-*s  %-8s %-12s %s", width, t.ID, t.State, t.Status, t.Title))
	}
	return strings.Join(lines, "\n"), nil
}

// TaskDetail renders one task (frontmatter fields + body).
func TaskDetail(task *models.Task, plain, asJSON bool) (string, error) {
	if asJSON {
		return marshalJSON(task.ToMap())
	}
	fields := []string{
		fmt.Sprintf("id:         %s", task.ID),
		fmt.Sprintf("title:      %s", task.Title),
		fmt.Sprintf("state:      %s", task.State),
		fmt.Sprintf("status:     %s", task.Status),
		fmt.Sprintf("agent:      %s", task.Agent),
		fmt.Sprintf("labels:     %s", strings.Join(task.Labels, ", ")),
		fmt.Sprintf("depends_on: %s", strings.Join(task.DependsOn, ", ")),
		fmt.Sprintf("created_at: %s", isoOrEmpty(task.CreatedAt)),
		fmt.Sprintf("updated_at: %s", isoOrEmpty(task.UpdatedAt)),
	}
	head := strings.Join(fields, "\n")
	body := strings.TrimSpace(task.Body)
	if body == "" {
		return head, nil
	}
	if plain {
		return fmt.Sprintf("%s\n\n%s", head, body), nil
	}
	return fmt.Sprintf("%s\n\n--- body ---\n%s", head, body), nil
}

// Board renders a board summary: per-status counts plus draft/archive totals.
func Board(counts []tasks.StatusCount, drafts, archived int, plain, asJSON bool) (string, error) {
	if asJSON {
		active := make(map[string]int, len(counts))
		for _, c := range counts {
			active[c.Status] = c.Count
		}
		return marshalJSON(map[string]any{
			"active":  active,
			"drafts":  drafts,
			"archive": archived,
		})
	}
	if plain {
		var lines []string
		for _, c := range counts {
			lines = append(lines, fmt.Sprintf("%s\t%d", c.Status, c.Count))
		}
		lines = append(lines, fmt.Sprintf("drafts\t%d", drafts), fmt.Sprintf("archive\t%d", archived))
		return strings.Join(lines, "\n"), nil
	}
	width := len("in_progress")
	total := 0
	for _, c := range counts {
		total += c.Count
	}
	var lines []string
	lines = append(lines, "active:")
	for _, c := range counts {
		lines = append(lines, fmt.Sprintf("  %-*s  %d", width, c.Status, c.Count))
	}
	lines = append(lines, fmt.Sprintf("  %-*s  %d", width, "total", total))
	lines = append(lines, fmt.Sprintf("draft: %d   archive: %d", drafts, archived))
	return strings.Join(lines, "\n"), nil
}

func summary(t *models.Task) map[string]any {
	m := t.ToMap()
	delete(m, "body")
	return m
}

func marshalJSON(v any) (string, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isoOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

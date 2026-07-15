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

// TaskList renders a list of tasks. In JSON mode the payload is an object
// carrying the tasks plus any board warnings; human/plain modes print tasks
// only (warnings go to stderr in the CLI). The --plain columns are
// id, state, status, assignee, labels (comma-joined), title — empty when
// unset, title always last.
func TaskList(taskList []*models.Task, warnings []tasks.Warning, plain, asJSON bool) (string, error) {
	if asJSON {
		summaries := make([]map[string]any, len(taskList))
		for i, t := range taskList {
			summaries[i] = summary(t)
		}
		return marshalJSON(map[string]any{
			"tasks":    summaries,
			"warnings": warningStrings(warnings),
		})
	}
	items := taskList
	if plain {
		lines := make([]string, len(items))
		for i, t := range items {
			lines[i] = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s",
				t.ID, t.State, t.Status, t.Assignee, strings.Join(t.Labels, ","), t.Title)
		}
		return strings.Join(lines, "\n"), nil
	}
	if len(items) == 0 {
		return "(no tasks)", nil
	}
	width := 0
	for _, t := range items {
		if len(t.ID) > width {
			width = len(t.ID)
		}
	}
	var lines []string
	for _, t := range items {
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
		fmt.Sprintf("assignee:   %s", task.Assignee),
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
// JSON mode also carries any board warnings.
func Board(counts []tasks.StatusCount, drafts, archived int, warnings []tasks.Warning, plain, asJSON bool) (string, error) {
	if asJSON {
		active := make(map[string]int, len(counts))
		for _, c := range counts {
			active[c.Status] = c.Count
		}
		return marshalJSON(map[string]any{
			"active":   active,
			"drafts":   drafts,
			"archive":  archived,
			"warnings": warningStrings(warnings),
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

// CleanupReport renders a cleanup run's archived (or would-be-archived) tasks.
// Human/plain match TaskList; JSON additionally carries "dry_run" so agents
// can confirm from the payload alone whether archiving actually happened.
func CleanupReport(archived []*models.Task, dryRun, plain, asJSON bool) (string, error) {
	if asJSON {
		summaries := make([]map[string]any, len(archived))
		for i, t := range archived {
			summaries[i] = summary(t)
		}
		return marshalJSON(map[string]any{
			"tasks":    summaries,
			"dry_run":  dryRun,
			"warnings": []string{},
		})
	}
	return TaskList(archived, nil, plain, false)
}

// DoctorReport renders doctor issues. Human/plain: one line per issue ("board
// is healthy" when clean); JSON: {"issues": [...]} with structured fields.
func DoctorReport(issues []tasks.Issue, plain, asJSON bool) (string, error) {
	if asJSON {
		out := make([]map[string]any, len(issues))
		for i, is := range issues {
			out[i] = map[string]any{
				"kind":   is.Kind,
				"file":   is.File,
				"detail": is.Detail,
				"fixed":  is.Fixed,
			}
		}
		return marshalJSON(map[string]any{"issues": out})
	}
	if len(issues) == 0 {
		if plain {
			return "ok", nil
		}
		return "Board is healthy — no issues found.", nil
	}
	lines := make([]string, len(issues))
	for i, is := range issues {
		lines[i] = is.String()
	}
	return strings.Join(lines, "\n"), nil
}

// warningStrings flattens warnings to their string form (never nil, so JSON
// renders [] rather than null).
func warningStrings(warnings []tasks.Warning) []string {
	out := make([]string, len(warnings))
	for i, w := range warnings {
		out[i] = w.String()
	}
	return out
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

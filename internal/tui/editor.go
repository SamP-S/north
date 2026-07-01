package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SamP-S/north/internal/models"
)

type editorMode int

const (
	modeCreate editorMode = iota
	modeEdit
)

// editorDoneMsg is sent after $EDITOR exits — the tmp file path is included so
// the model can read and parse the result.
type editorDoneMsg struct {
	tmpPath string
	mode    editorMode
	taskID  string // only set for modeEdit
}

// createTemplate returns a blank task template for $EDITOR.
func createTemplate() string {
	return "# Task title here\nagent: \nlabels: \ndepends_on: \n\nDescribe the task body here.\n"
}

// taskToTemplate serialises a task into the editor template format.
func taskToTemplate(t *models.Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n", t.Title)
	fmt.Fprintf(&sb, "agent: %s\n", t.Agent)
	fmt.Fprintf(&sb, "labels: %s\n", strings.Join(t.Labels, ", "))
	fmt.Fprintf(&sb, "depends_on: %s\n", strings.Join(t.DependsOn, ", "))
	sb.WriteString("\n")
	if t.Body != "" {
		sb.WriteString(t.Body)
		if !strings.HasSuffix(t.Body, "\n") {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// ParseEditorResult parses the editor template and returns title, body, agent,
// labels, and depends_on. Exported so tests can call it directly.
//
// Format:
//
//	# Title line
//	agent: name             (optional)
//	labels: foo, bar        (optional)
//	depends_on: task-1, task-2   (optional)
//
//	Body text…
func ParseEditorResult(content string) (title, body, agent string, labels, dependsOn []string) {
	lines := strings.Split(content, "\n")
	inHeader := true
	var bodyLines []string

	for _, line := range lines {
		if inHeader {
			if strings.HasPrefix(line, "# ") && title == "" {
				title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				continue
			}
			if strings.HasPrefix(line, "agent:") && title != "" {
				agent = strings.TrimSpace(strings.TrimPrefix(line, "agent:"))
				continue
			}
			if strings.HasPrefix(line, "labels:") && title != "" {
				labels = splitCommaList(strings.TrimPrefix(line, "labels:"))
				continue
			}
			if strings.HasPrefix(line, "depends_on:") && title != "" {
				dependsOn = splitCommaList(strings.TrimPrefix(line, "depends_on:"))
				continue
			}
			// blank line after header ends the header block
			if line == "" && title != "" {
				inHeader = false
				continue
			}
			// any non-header content while title is set ends the header
			if title != "" {
				inHeader = false
				bodyLines = append(bodyLines, line)
			}
		} else {
			bodyLines = append(bodyLines, line)
		}
	}

	body = strings.TrimRight(strings.Join(bodyLines, "\n"), "\n")
	return
}

// splitCommaList trims a "field: a, b, c" value (with the "field:" prefix
// already stripped) into its non-empty, whitespace-trimmed elements.
func splitCommaList(raw string) []string {
	var out []string
	for _, v := range strings.Split(strings.TrimSpace(raw), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// editorFor returns the user's preferred editor ($EDITOR, $VISUAL, or vi).
func editorFor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	return "vi"
}

// openEditor writes content to a temp file, then returns a tea.ExecProcess Cmd
// that suspends the TUI, opens $EDITOR on the file, and resumes with an
// editorDoneMsg (or errMsg if setup fails).
//
// Call this from Update — the returned Cmd must be returned alongside the model.
func openEditor(content string, mode editorMode, taskID string) tea.Cmd {
	f, err := os.CreateTemp("", "north-task-*.md")
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	path := f.Name()
	if _, werr := f.WriteString(content); werr != nil {
		f.Close()
		os.Remove(path)
		return func() tea.Msg { return errMsg{werr} }
	}
	f.Close()

	cmd := exec.Command(editorFor(), path) //nolint:gosec
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			// non-zero exit (e.g. :cq in vim) — treat as cancelled, still try to
			// read what was saved
		}
		return editorDoneMsg{tmpPath: path, mode: mode, taskID: taskID}
	})
}

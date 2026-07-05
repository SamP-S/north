// editor.go — the $EDITOR flow for creating and editing tasks.
//
// The editor buffer uses the real on-disk task-file format (YAML frontmatter +
// Markdown body), restricted to the editable fields: title, agent, labels,
// depends_on, and the body. id/state/status are managed by their own keys and
// never appear in the buffer. Parsing reuses the same frontmatter code as the
// task files themselves.
package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

type editorMode int

const (
	modeCreate editorMode = iota
	modeEdit
)

// editorDoneMsg is sent after $EDITOR exits — the tmp file path is included so
// the model can read and parse the result. canceled is set when the editor
// exited non-zero (e.g. vim's :cq), which aborts the operation.
type editorDoneMsg struct {
	tmpPath  string
	mode     editorMode
	taskID   string // only set for modeEdit
	canceled bool
}

// createTemplate returns a blank task skeleton for $EDITOR.
func createTemplate() string {
	out, err := tasks.RenderEditorDoc(tasks.EditorDoc{})
	if err != nil {
		return "---\ntitle: \nagent: \nlabels: []\ndepends_on: []\n---\n\n"
	}
	return out + "Describe the task here.\n"
}

// editTemplate serialises a task's editable fields into the editor format.
func editTemplate(t *models.Task) (string, error) {
	return tasks.RenderEditorDoc(tasks.EditorDoc{
		Title:     t.Title,
		Agent:     t.Agent,
		Labels:    t.Labels,
		DependsOn: t.DependsOn,
		Body:      t.Body,
	})
}

// editorCommand resolves the user's editor ($VISUAL, then $EDITOR, then vi)
// and builds the exec.Cmd. Values with arguments ("code --wait") are split.
func editorCommand(path string) *exec.Cmd {
	for _, v := range []string{os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if fields := strings.Fields(v); len(fields) > 0 {
			return exec.Command(fields[0], append(fields[1:], path)...) //nolint:gosec
		}
	}
	return exec.Command("vi", path)
}

// openEditor writes content to a temp file, then returns a tea.ExecProcess Cmd
// that suspends the TUI, opens the editor on the file, and resumes with an
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

	return tea.ExecProcess(editorCommand(path), func(err error) tea.Msg {
		// Non-zero exit (e.g. :cq in vim) means "abort, discard my edits".
		return editorDoneMsg{tmpPath: path, mode: mode, taskID: taskID, canceled: err != nil}
	})
}

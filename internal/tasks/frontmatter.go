// frontmatter.go — task-file parsing and serialisation.
//
// Task files are YAML frontmatter ("---" fenced) followed by a free-form
// Markdown body. Parsing works on yaml.Node so that unknown frontmatter keys
// (and their order) survive a read-modify-write cycle untouched — North only
// overlays the keys it owns. CRLF line endings are tolerated on read; files
// are always written with LF.
package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/SamP-S/north/internal/models"
	"gopkg.in/yaml.v3"
)

// taskFields is the typed view of the frontmatter keys North owns. All scalar
// values are coerced to strings via the raw node value, so `id: 12` and
// `depends_on: [4]` parse the same as their quoted forms.
type taskFields struct {
	ID        string
	Title     string
	Status    string
	Assignee  string
	Labels    []string
	DependsOn []string
	CreatedAt string
	UpdatedAt string
}

// normalizeNewlines converts CRLF (and stray CR) to LF.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// splitFrontmatter separates a leading "---\n…\n---" YAML block from the body.
// Content must already be newline-normalized.
func splitFrontmatter(content string) (meta, body string, err error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---\n") && content != "---" {
		// No frontmatter: whole thing is body.
		return "", content, nil
	}
	rest := strings.TrimPrefix(content, "---\n")
	if rest == content {
		rest = strings.TrimPrefix(content, "---")
	}
	// The closing fence is a line that is exactly "---" plus optional
	// trailing spaces/tabs, terminated by "\n" or EOF. "----" and "--- text"
	// do not terminate the block.
	for pos := 0; ; {
		lineEnd := strings.IndexByte(rest[pos:], '\n')
		var line string
		if lineEnd < 0 {
			line = rest[pos:]
		} else {
			line = rest[pos : pos+lineEnd]
		}
		if closingFence(line) {
			meta = strings.TrimSuffix(rest[:pos], "\n")
			if lineEnd < 0 {
				return meta, "", nil
			}
			return meta, rest[pos+lineEnd+1:], nil
		}
		if lineEnd < 0 {
			return "", "", fmt.Errorf("unterminated frontmatter block")
		}
		pos += lineEnd + 1
	}
}

// closingFence reports whether a line ends a frontmatter block: exactly "---"
// plus optional trailing spaces/tabs.
func closingFence(line string) bool {
	return strings.HasPrefix(line, "---") && strings.TrimRight(line[3:], " \t") == ""
}

// parseFront unmarshals a frontmatter block into its document node (for
// round-tripping) and the typed fields North knows about.
func parseFront(meta string) (*yaml.Node, taskFields, error) {
	var f taskFields
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(meta), &doc); err != nil {
		return nil, f, err
	}
	m := mappingOf(&doc)
	if m == nil {
		return &doc, f, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		switch k.Value {
		case "id":
			f.ID = scalarString(v)
		case "title":
			f.Title = scalarString(v)
		case "status":
			f.Status = scalarString(v)
		case "assignee":
			f.Assignee = scalarString(v)
		case "labels":
			f.Labels = seqStrings(v)
		case "depends_on":
			f.DependsOn = seqStrings(v)
		case "created_at":
			f.CreatedAt = scalarString(v)
		case "updated_at":
			f.UpdatedAt = scalarString(v)
		}
	}
	return &doc, f, nil
}

// renderTask serialises a task back to file content. prev is the previously
// parsed frontmatter document (nil for new tasks); unknown keys in it are
// preserved in place while North's own keys are overlaid.
func renderTask(task *models.Task, prev *yaml.Node) (string, error) {
	m := mappingOf(prev)
	if m == nil {
		m = &yaml.Node{Kind: yaml.MappingNode}
	}
	setKey(m, "id", strScalar(task.ID, true))
	setKey(m, "title", strScalar(task.Title, false))
	setKey(m, "status", strScalar(string(task.Status), false))
	setKey(m, "assignee", strScalar(task.Assignee, false))
	setKey(m, "labels", seqScalar(task.Labels, false))
	setKey(m, "depends_on", seqScalar(task.DependsOn, true))
	setKey(m, "created_at", timeScalar(task.CreatedAt))
	setKey(m, "updated_at", timeScalar(task.UpdatedAt))

	out, err := yaml.Marshal(m)
	if err != nil {
		return "", err
	}
	front := strings.TrimRight(string(out), "\n")
	body := strings.Trim(task.Body, "\n")
	if body != "" {
		return fmt.Sprintf("---\n%s\n---\n\n%s\n", front, body), nil
	}
	return fmt.Sprintf("---\n%s\n---\n", front), nil
}

// mappingOf returns the top-level mapping node of a parsed YAML document, or
// nil when the document is empty or not a mapping.
func mappingOf(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
			return doc.Content[0]
		}
		return nil
	}
	if doc.Kind == yaml.MappingNode {
		return doc
	}
	return nil
}

// scalarString returns a scalar node's raw value ("" for null/non-scalars).
func scalarString(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		return ""
	}
	return n.Value
}

// seqStrings returns a sequence node's scalar values as strings.
func seqStrings(n *yaml.Node) []string {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, c := range n.Content {
		if s := scalarString(c); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// setKey replaces key's value in a mapping node, appending the pair when the
// key is missing (so fresh files get keys in call order and existing files
// keep their layout).
func setKey(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
}

// strScalar builds a string scalar node. quoted forces double quotes so
// numeric-looking values (bare ids, timestamps) stay strings when re-read.
func strScalar(s string, quoted bool) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if quoted {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}

// seqScalar builds a flow-style sequence node of string scalars.
func seqScalar(vals []string, quoted bool) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, v := range vals {
		n.Content = append(n.Content, strScalar(v, quoted))
	}
	return n
}

// timeScalar builds a quoted RFC3339 scalar node (null when t is nil).
func timeScalar(t *time.Time) *yaml.Node {
	if t == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	return strScalar(t.Format(time.RFC3339), true)
}

// EditorDoc is the editable subset of a task exchanged with the TUI's $EDITOR
// flow: the same on-disk format (YAML frontmatter + body), restricted to the
// fields the editor may change. id/state/status are managed by their own
// commands and are deliberately absent.
type EditorDoc struct {
	Title     string
	Assignee  string
	Labels    []string
	DependsOn []string
	Body      string
}

// RenderEditorDoc serialises an EditorDoc to frontmatter+body file content.
func RenderEditorDoc(d EditorDoc) (string, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	setKey(m, "title", strScalar(d.Title, false))
	setKey(m, "assignee", strScalar(d.Assignee, false))
	setKey(m, "labels", seqScalar(d.Labels, false))
	setKey(m, "depends_on", seqScalar(d.DependsOn, true))
	out, err := yaml.Marshal(m)
	if err != nil {
		return "", err
	}
	front := strings.TrimRight(string(out), "\n")
	body := strings.Trim(d.Body, "\n")
	if body != "" {
		return fmt.Sprintf("---\n%s\n---\n\n%s\n", front, body), nil
	}
	return fmt.Sprintf("---\n%s\n---\n\n", front), nil
}

// ParseEditorDoc parses editor output written in the task-file format.
func ParseEditorDoc(content string) (EditorDoc, error) {
	var d EditorDoc
	meta, body, err := splitFrontmatter(normalizeNewlines(content))
	if err != nil {
		return d, err
	}
	_, f, err := parseFront(meta)
	if err != nil {
		return d, err
	}
	d.Title = f.Title
	d.Assignee = f.Assignee
	d.Labels = f.Labels
	d.DependsOn = f.DependsOn
	d.Body = strings.Trim(body, "\n")
	return d, nil
}

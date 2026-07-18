package tasks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

// writeRaw drops a hand-written task file into a state folder, bypassing the
// normal Create path so the parser can be exercised against bad input.
func writeRaw(t *testing.T, boardDir, stateFolder, filename, content string) {
	t.Helper()
	p := filepath.Join(boardDir, stateFolder, filename)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadToleratesMalformedFiles(t *testing.T) {
	cases := []struct {
		name, file, content string
	}{
		{"unterminated frontmatter", "1-a.md", "---\nid: \"1\"\ntitle: a\n"},
		{"missing id", "2-a.md", "---\ntitle: a\nstatus: ready\n---\n"},
		{"missing title", "3-a.md", "---\nid: \"3\"\nstatus: ready\n---\n"},
		{"unknown status", "4-a.md", "---\nid: \"4\"\ntitle: a\nstatus: bogus\n---\n"},
		{"broken yaml", "5-a.md", "---\nid: [unclosed\n---\n"},
		{"traversal id", "6-a.md", "---\nid: \"../../evil\"\ntitle: a\nstatus: ready\n---\n"},
		{"non-numeric id", "7-a.md", "---\nid: seven\ntitle: a\nstatus: ready\n---\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			boardDir := newBoard(t)
			mustCreate(t, boardDir, "good one")
			writeRaw(t, boardDir, "tasks", c.file, c.content)
			// One bad file must never abort the load — it becomes a warning
			// naming the file, and the good task still lists.
			snap, err := tasks.Load(boardDir)
			if err != nil {
				t.Fatalf("tolerant load errored: %v", err)
			}
			if len(snap.Tasks) != 1 {
				t.Errorf("good task lost: %v", snap.Tasks)
			}
			if len(snap.Warnings) != 1 || !strings.Contains(snap.Warnings[0].String(), c.file) {
				t.Errorf("expected one warning naming %s, got %v", c.file, snap.Warnings)
			}
		})
	}
}

func TestLoadAcceptsCRLF(t *testing.T) {
	boardDir := newBoard(t)
	writeRaw(t, boardDir, "tasks", "6-crlf.md",
		"---\r\nid: \"6\"\r\ntitle: windows file\r\nstatus: ready\r\n---\r\nbody line\r\n")
	task, err := tasks.Get(boardDir, "6")
	if err != nil {
		t.Fatalf("CRLF file should parse: %v", err)
	}
	if task.Title != "windows file" || task.Body != "body line" {
		t.Errorf("CRLF parse mangled fields: %+v", task)
	}
}

func TestLoadCoercesScalars(t *testing.T) {
	boardDir := newBoard(t)
	// Unquoted numeric id and deps, as a human might hand-write them.
	writeRaw(t, boardDir, "tasks", "7-hand.md",
		"---\nid: 7\ntitle: hand written\nstatus: in_progress\ndepends_on: [7]\n---\n\nbody text\n")
	task, err := tasks.Get(boardDir, "7")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != models.StateActive || task.Status != models.InProgress {
		t.Errorf("unexpected: state=%s status=%s", task.State, task.Status)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "7" {
		t.Errorf("numeric depends_on not coerced: %v", task.DependsOn)
	}
	if task.Body != "body text" {
		t.Errorf("body: %q", task.Body)
	}
}

func TestFrontmatterFenceIsExact(t *testing.T) {
	boardDir := newBoard(t)
	// A meta line starting with "----" is not a closing fence: everything up
	// to the real "---" stays in the frontmatter.
	writeRaw(t, boardDir, "tasks", "9-dashes.md",
		"---\nid: \"9\"\ntitle: dashes\nstatus: ready\n----: marker\nassignee: sam\n---\nbody here\n")
	task, err := tasks.Get(boardDir, "9")
	if err != nil {
		t.Fatalf("meta line starting with ---- broke the parse: %v", err)
	}
	if task.Assignee != "sam" || task.Body != "body here" {
		t.Errorf("frontmatter cut short at ----: %+v", task)
	}

	// A closing fence with trailing spaces still terminates the block.
	writeRaw(t, boardDir, "tasks", "10-trailing.md",
		"---\nid: \"10\"\ntitle: trailing\nstatus: ready\n---  \nbody here\n")
	task, err = tasks.Get(boardDir, "10")
	if err != nil {
		t.Fatalf("trailing spaces on fence should parse: %v", err)
	}
	if task.Body != "body here" {
		t.Errorf("body after padded fence: %q", task.Body)
	}

	// A block "closed" only by "----" is unterminated.
	boardDir2 := newBoard(t)
	writeRaw(t, boardDir2, "tasks", "1-bad.md",
		"---\nid: \"1\"\ntitle: bad\nstatus: ready\n----\n")
	snap, err := tasks.Load(boardDir2)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Warnings) != 1 || !strings.Contains(snap.Warnings[0].Err, "unterminated frontmatter") {
		t.Errorf("expected unterminated-frontmatter warning, got %v", snap.Warnings)
	}
}

func TestLoadRejectsTraversalID(t *testing.T) {
	boardDir := newBoard(t)
	writeRaw(t, boardDir, "tasks", "1-evil.md",
		"---\nid: \"../../evil\"\ntitle: evil\nstatus: ready\n---\n")
	// The id is interpolated into filenames, so a non-numeric id must be an
	// Invalid parse failure naming the file — never a loadable task.
	snap, err := tasks.Load(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 0 {
		t.Errorf("traversal id loaded as a task: %v", snap.Tasks)
	}
	if len(snap.Warnings) != 1 ||
		!strings.Contains(snap.Warnings[0].Err, "failed to parse 1-evil.md") ||
		!strings.Contains(snap.Warnings[0].Err, "not a bare number") {
		t.Errorf("expected invalid-id warning naming the file, got %v", snap.Warnings)
	}
}

func TestFrontmatterFenceEdges(t *testing.T) {
	boardDir := newBoard(t)

	// A leading UTF-8 BOM before the opening fence still parses.
	writeRaw(t, boardDir, "tasks", "11-bom.md",
		string(rune(0xFEFF))+"---\nid: \"11\"\ntitle: bom\nstatus: ready\n---\nbody here\n")
	task, err := tasks.Get(boardDir, "11")
	if err != nil {
		t.Fatalf("BOM before fence should parse: %v", err)
	}
	if task.Title != "bom" || task.Body != "body here" {
		t.Errorf("BOM parse mangled fields: %+v", task)
	}

	// A closing fence at EOF with no trailing newline still terminates.
	writeRaw(t, boardDir, "tasks", "12-eof.md",
		"---\nid: \"12\"\ntitle: eof fence\nstatus: ready\n---")
	task, err = tasks.Get(boardDir, "12")
	if err != nil {
		t.Fatalf("fence at EOF should parse: %v", err)
	}
	if task.Body != "" {
		t.Errorf("body after EOF fence should be empty: %q", task.Body)
	}

	// A body whose first line is "---" stays in the body: the first fence
	// after the meta closes the block, later ones never extend it.
	writeRaw(t, boardDir, "tasks", "13-hr.md",
		"---\nid: \"13\"\ntitle: hr body\nstatus: ready\n---\n---\nafter rule\n")
	task, err = tasks.Get(boardDir, "13")
	if err != nil {
		t.Fatal(err)
	}
	if task.Body != "---\nafter rule" {
		t.Errorf("body-leading --- mangled: %q", task.Body)
	}

	// Empty meta ("---\n---\nbody") is a valid fence pair — the load fails
	// on the empty status field, never on the frontmatter block itself.
	boardDir2 := newBoard(t)
	writeRaw(t, boardDir2, "tasks", "1-empty.md", "---\n---\nbody\n")
	snap, err := tasks.Load(boardDir2)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Warnings) != 1 || !strings.Contains(snap.Warnings[0].Err, "unknown status") {
		t.Errorf("empty meta should fail on the status field, got %v", snap.Warnings)
	}
}

func TestUnknownFrontmatterKeysSurviveRewrite(t *testing.T) {
	boardDir := newBoard(t)
	writeRaw(t, boardDir, "tasks", "8-custom.md",
		"---\nid: \"8\"\ntitle: custom fields\nstatus: ready\npriority: high\nowner: sam\n---\nbody\n")
	// A status change rewrites the file in place…
	if _, _, err := tasks.SetStatus(boardDir, "8", "in_progress"); err != nil {
		t.Fatal(err)
	}
	// …and an edit renames it; unknown keys must survive both.
	title := "renamed"
	if _, _, err := tasks.Edit(boardDir, "8", tasks.EditOpts{Title: &title}); err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Get(boardDir, "8")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(task.Path)
	if !strings.Contains(string(data), "priority: high") || !strings.Contains(string(data), "owner: sam") {
		t.Errorf("unknown keys lost on rewrite:\n%s", data)
	}
	if !strings.Contains(string(data), "status: in_progress") || !strings.Contains(string(data), "title: renamed") {
		t.Errorf("owned keys not updated:\n%s", data)
	}
}

func TestSnapshotFlagsDuplicateIDs(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "original") // 1
	// Simulate a git merge that brought in a second file with the same id.
	writeRaw(t, boardDir, "drafts", "1-merged-copy.md",
		"---\nid: \"1\"\ntitle: merged copy\nstatus: ready\n---\n")
	snap, err := tasks.Load(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 1 {
		t.Errorf("duplicate should be excluded from tasks: %v", snap.Tasks)
	}
	if len(snap.Warnings) != 1 || !strings.Contains(snap.Warnings[0].Err, "duplicate id") {
		t.Errorf("expected duplicate-id warning, got %v", snap.Warnings)
	}
}

func TestRoundTripFidelity(t *testing.T) {
	boardDir := newBoard(t)
	// Pre-create the dep so validateDeps is satisfied.
	mustCreate(t, boardDir, "dep") // 1
	// Unicode title and a body containing a literal "---" line.
	title := "Café — déjà vu"
	body := "first line\n\n---\na horizontal rule inside the body\n---\n\nlast line"
	created, _, err := tasks.Create(boardDir, title, "ollama:llama3", []string{"x", "y"}, []string{"1"}, body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tasks.Get(boardDir, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != title {
		t.Errorf("title: %q != %q", got.Title, title)
	}
	if got.Body != body {
		t.Errorf("body round-trip failed:\n got %q\nwant %q", got.Body, body)
	}
	if got.Assignee != "ollama:llama3" || len(got.Labels) != 2 || len(got.DependsOn) != 1 {
		t.Errorf("fields lost: %+v", got)
	}
	// Timestamps persist at RFC3339 (second) precision.
	if got.CreatedAt == nil || got.CreatedAt.Format(time.RFC3339) != created.CreatedAt.Format(time.RFC3339) {
		t.Errorf("created_at not preserved: got %v want %v", got.CreatedAt, created.CreatedAt)
	}
}

func TestEditorDocRoundTrip(t *testing.T) {
	doc := tasks.EditorDoc{
		Title:     "Fix the télé",
		Assignee:  "opus",
		Labels:    []string{"a", "b"},
		DependsOn: []string{"4"},
		Body:      "line one\n\nline two",
	}
	out, err := tasks.RenderEditorDoc(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tasks.ParseEditorDoc(out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != doc.Title || got.Assignee != doc.Assignee || got.Body != doc.Body {
		t.Errorf("round trip: %+v != %+v", got, doc)
	}
	if len(got.Labels) != 2 || len(got.DependsOn) != 1 || got.DependsOn[0] != "4" {
		t.Errorf("lists lost: %+v", got)
	}
}

package tasks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/tasks"
)

func issueKinds(issues []tasks.Issue) map[string]int {
	out := map[string]int{}
	for _, i := range issues {
		out[i.Kind]++
	}
	return out
}

func TestDoctorCleanBoard(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "fine")
	issues, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("expected clean board, got %v", issues)
	}
}

func TestDoctorGitattributes(t *testing.T) {
	boardDir := newBoard(t)
	if err := os.Remove(filepath.Join(boardDir, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	issues, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if issueKinds(issues)["gitattributes"] != 1 {
		t.Fatalf("missing .gitattributes not reported: %v", issues)
	}
	// --fix restores it.
	issues, err = tasks.Doctor(boardDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !issues[0].Fixed {
		t.Errorf("fix should restore .gitattributes: %v", issues)
	}
	if _, err := os.Stat(filepath.Join(boardDir, ".gitattributes")); err != nil {
		t.Errorf(".gitattributes not restored: %v", err)
	}
}

func TestDoctorFlagsUnknownStatus(t *testing.T) {
	boardDir := newBoard(t)
	// A hand-edited status outside the fixed set is unparseable, not silent.
	writeRaw(t, boardDir, "tasks", "1-weird.md",
		"---\nid: \"1\"\ntitle: weird\nstatus: someday\n---\n")
	issues, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if issueKinds(issues)["unparseable"] != 1 {
		t.Errorf("unknown status should surface as unparseable: %v", issues)
	}
}

func TestDoctorDetects(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "original") // 1

	// Duplicate id (merge artefact).
	writeRaw(t, boardDir, "drafts", "1-merged.md",
		"---\nid: \"1\"\ntitle: merged\nstatus: ready\n---\n")
	// CRLF file.
	writeRaw(t, boardDir, "tasks", "2-crlf.md",
		"---\r\nid: \"2\"\r\ntitle: crlf\r\nstatus: ready\r\n---\r\n")
	// Unparseable file.
	writeRaw(t, boardDir, "tasks", "3-broken.md", "garbage, no frontmatter")
	// Dangling dep + cycle: 4 → 5 → 4.
	writeRaw(t, boardDir, "tasks", "4-a.md",
		"---\nid: \"4\"\ntitle: a\nstatus: ready\ndepends_on: [\"5\", \"404\"]\n---\n")
	writeRaw(t, boardDir, "tasks", "5-b.md",
		"---\nid: \"5\"\ntitle: b\nstatus: ready\ndepends_on: [\"4\"]\n---\n")
	// Filename/frontmatter drift.
	writeRaw(t, boardDir, "archive", "9-drift.md",
		"---\nid: \"6\"\ntitle: drifted\nstatus: ready\n---\n")

	issues, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	kinds := issueKinds(issues)
	for _, want := range []string{"duplicate-id", "crlf", "unparseable", "dangling-dep", "cycle", "id-drift"} {
		if kinds[want] == 0 {
			t.Errorf("missing %s issue; got %v", want, issues)
		}
	}
	// Detect-only must not modify anything.
	if _, err := os.Stat(filepath.Join(boardDir, "drafts", "1-merged.md")); err != nil {
		t.Errorf("detect mode moved files: %v", err)
	}
}

func TestDoctorFix(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "original") // 1
	writeRaw(t, boardDir, "drafts", "1-merged.md",
		"---\nid: \"1\"\ntitle: merged\nstatus: ready\n---\n")
	writeRaw(t, boardDir, "tasks", "2-crlf.md",
		"---\r\nid: \"2\"\r\ntitle: crlf\r\nstatus: ready\r\n---\r\n")

	issues, err := tasks.Doctor(boardDir, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range issues {
		if !i.Fixed {
			t.Errorf("issue not fixed: %v", i)
		}
	}

	// The board must now be healthy.
	again, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("issues remain after --fix: %v", again)
	}

	// The duplicate got a fresh id (3: max was 2 after the CRLF file).
	snap, _ := tasks.Load(boardDir)
	if len(snap.Warnings) != 0 {
		t.Errorf("warnings remain: %v", snap.Warnings)
	}
	// One of the two id-1 files kept the id, the other moved to 3.
	if snap.Get("1") == nil || snap.Get("3") == nil {
		ids := make([]string, len(snap.Tasks))
		for i, task := range snap.Tasks {
			ids[i] = task.ID
		}
		t.Errorf("duplicate not renumbered to 3: ids %v", ids)
	}
	// CRLF file rewritten with LF.
	data, _ := os.ReadFile(filepath.Join(boardDir, "tasks", "2-crlf.md"))
	if strings.Contains(string(data), "\r") {
		t.Error("CRLF not rewritten")
	}
}

func TestDoctorFixDriftedDuplicate(t *testing.T) {
	boardDir := newBoard(t)
	mustCreate(t, boardDir, "original") // 1 (drafts)
	// One file with both problems: its filename id drifts from its frontmatter
	// id, and that frontmatter id duplicates task 1. The drift repair renames
	// the file, so the duplicate repair must load the renamed path.
	writeRaw(t, boardDir, "tasks", "9-drift-dup.md",
		"---\nid: \"1\"\ntitle: drifted dup\nstatus: ready\n---\n")

	issues, err := tasks.Doctor(boardDir, true)
	if err != nil {
		t.Fatal(err)
	}
	kinds := issueKinds(issues)
	if kinds["id-drift"] != 1 || kinds["duplicate-id"] != 1 {
		t.Fatalf("expected one id-drift and one duplicate-id, got %v", issues)
	}
	for _, i := range issues {
		if !i.Fixed {
			t.Errorf("issue not fixed in one run: %v", i)
		}
	}

	// A single --fix run must leave the board healthy.
	again, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("issues remain after --fix: %v", again)
	}
	// The drifted file was renumbered to a fresh id.
	snap, _ := tasks.Load(boardDir)
	if snap.Get("1") == nil || snap.Get("2") == nil {
		ids := make([]string, len(snap.Tasks))
		for i, task := range snap.Tasks {
			ids[i] = task.ID
		}
		t.Errorf("duplicate not renumbered: ids %v", ids)
	}
}

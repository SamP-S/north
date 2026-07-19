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

func TestDoctorFixDriftedDuplicateSameSlugNoClobber(t *testing.T) {
	boardDir := newBoard(t)
	// The legitimate task and a drifted duplicate share the same title slug,
	// so the drift repair's rename target ("1-foo.md") is already taken.
	// Renaming would silently overwrite the original; instead the drifted
	// file must keep its name and be renumbered by the duplicate-id repair,
	// which writes the new id's filename and heals the drift as a side
	// effect — a single --fix run leaves the board clean with no data loss.
	writeRaw(t, boardDir, "tasks", "1-foo.md",
		"---\nid: \"1\"\ntitle: foo\nstatus: ready\n---\noriginal body\n")
	writeRaw(t, boardDir, "tasks", "9-foo.md",
		"---\nid: \"1\"\ntitle: foo\nstatus: ready\n---\nduplicate body\n")

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

	// The original file survives with its content intact.
	data, err := os.ReadFile(filepath.Join(boardDir, "tasks", "1-foo.md"))
	if err != nil {
		t.Fatalf("original file clobbered: %v", err)
	}
	if !strings.Contains(string(data), "original body") {
		t.Errorf("original content lost:\n%s", data)
	}
	// No task lost: the duplicate was renumbered past the filename scan (9).
	snap, err := tasks.Load(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 2 || snap.Get("1") == nil || snap.Get("10") == nil {
		ids := make([]string, len(snap.Tasks))
		for i, task := range snap.Tasks {
			ids[i] = task.ID
		}
		t.Errorf("expected tasks 1 and 10, got ids %v", ids)
	}
	if dup := snap.Get("10"); dup != nil && dup.Body != "duplicate body" {
		t.Errorf("duplicate content lost: %q", dup.Body)
	}
	// A single --fix run must leave the board healthy.
	again, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("issues remain after --fix: %v", again)
	}
}

func TestDoctorFixDuplicateWithDanglingDepOneRun(t *testing.T) {
	boardDir := newBoard(t)
	// One file is BOTH a duplicate id AND carries a dangling depends_on. The
	// duplicate-id repair renumbers it (new path, new id); the dangling-dep
	// repair must then load the renamed file — not the removed original — so a
	// single --fix run heals both problems.
	writeRaw(t, boardDir, "tasks", "2-beta.md",
		"---\nid: \"2\"\ntitle: beta\nstatus: ready\n---\n")
	writeRaw(t, boardDir, "tasks", "2-dupe.md",
		"---\nid: \"2\"\ntitle: dupe\nstatus: ready\ndepends_on: [\"99\"]\n---\n")

	issues, err := tasks.Doctor(boardDir, true)
	if err != nil {
		t.Fatal(err)
	}
	var dupFixed, danglingFixed bool
	for _, i := range issues {
		switch i.Kind {
		case "duplicate-id":
			dupFixed = i.Fixed
		case "dangling-dep":
			danglingFixed = i.Fixed
		}
	}
	if !dupFixed {
		t.Errorf("duplicate-id not fixed in one run: %v", issues)
	}
	if !danglingFixed {
		t.Errorf("dangling-dep not fixed in one run (needs the renamed path): %v", issues)
	}

	// A second run must find nothing — the dangling dep did not survive.
	again, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("issues remain after one --fix run: %v", again)
	}
}

func TestDoctorFlagsBadTimestamp(t *testing.T) {
	boardDir := newBoard(t)
	// A hand-edited, unparseable created_at is report-only: doctor flags it but
	// --fix must not touch the value.
	writeRaw(t, boardDir, "tasks", "1-bad.md",
		"---\nid: \"1\"\ntitle: bad ts\nstatus: ready\ncreated_at: 24-06-2026 10:00\n---\n")

	issues, err := tasks.Doctor(boardDir, false)
	if err != nil {
		t.Fatal(err)
	}
	var badTS []tasks.Issue
	for _, i := range issues {
		if i.Kind == "bad-timestamp" {
			badTS = append(badTS, i)
		}
	}
	if len(badTS) != 1 {
		t.Fatalf("expected exactly one bad-timestamp issue, got %v", issues)
	}
	if badTS[0].File != "1-bad.md" || !strings.Contains(badTS[0].Detail, "created_at") {
		t.Errorf("bad-timestamp issue should name the file and key: %+v", badTS[0])
	}
	if badTS[0].Fixed {
		t.Errorf("bad-timestamp must be report-only: %+v", badTS[0])
	}

	// --fix must leave the value untouched (report-only) and still report it.
	if _, err := tasks.Doctor(boardDir, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(boardDir, "tasks", "1-bad.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "24-06-2026 10:00") {
		t.Errorf("report-only bad timestamp was modified:\n%s", data)
	}
}

func TestDoctorTraversalIDIsUnparseable(t *testing.T) {
	boardDir := newBoard(t)
	// A crafted frontmatter id must never reach a filename: the file is
	// unparseable (report-only), even under --fix.
	writeRaw(t, boardDir, "tasks", "1-evil.md",
		"---\nid: \"../../evil\"\ntitle: evil\nstatus: ready\n---\n")
	issues, err := tasks.Doctor(boardDir, true)
	if err != nil {
		t.Fatal(err)
	}
	kinds := issueKinds(issues)
	if kinds["unparseable"] != 1 || kinds["id-drift"] != 0 {
		t.Fatalf("traversal id should surface as unparseable, got %v", issues)
	}
	// The file stays put; nothing was written outside the board.
	if _, err := os.Stat(filepath.Join(boardDir, "tasks", "1-evil.md")); err != nil {
		t.Errorf("file moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(boardDir, "..", "evil-evil.md")); !os.IsNotExist(err) {
		t.Errorf("file escaped the board: %v", err)
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

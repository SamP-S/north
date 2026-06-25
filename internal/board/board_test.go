package board_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
)

// newBoard scaffolds a board in a fresh tmp repo and returns the board dir.
func newBoard(t *testing.T) string {
	t.Helper()
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return dir
}

func TestInitScaffoldsEverything(t *testing.T) {
	boardDir := newBoard(t)
	root := board.Root(boardDir)
	if _, err := os.Stat(filepath.Join(boardDir, "config.yml")); err != nil {
		t.Errorf("config.yml missing: %v", err)
	}
	// State folders exist.
	for _, state := range models.StateOrder {
		if fi, err := os.Stat(board.StateDir(boardDir, state)); err != nil || !fi.IsDir() {
			t.Errorf("state dir for %s missing", state)
		}
	}
	// No AGENTS.md is written any more.
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md should not be written by init")
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoCommit {
		t.Errorf("auto_commit should default false")
	}
}

func TestInitIsIdempotent(t *testing.T) {
	root := t.TempDir()
	boardDir, err := board.InitBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	// A custom config must survive a re-init.
	if _, err := board.WriteConfig(boardDir, board.Config{AutoCommit: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := board.InitBoard(root); err != nil {
		t.Fatal(err)
	}
	cfg, _ := board.LoadConfig(boardDir)
	if !cfg.AutoCommit {
		t.Errorf("re-init overwrote existing config")
	}
}

func TestLocateWalksUp(t *testing.T) {
	boardDir := newBoard(t)
	root := board.Root(boardDir)
	nested := filepath.Join(root, "src", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := board.LocateBoard(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != boardDir {
		t.Errorf("got %s want %s", got, boardDir)
	}
}

func TestLocateMissingRaises(t *testing.T) {
	_, err := board.LocateBoard(t.TempDir())
	if _, ok := nerrors.As(err); !ok {
		t.Fatalf("expected BoardError, got %v", err)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Add login form": "Add-login-form",
		"  spaced  ":     "spaced",
		"a/b:c":          "a-b-c",
		"Café déjà":      "Caf-d-j", // non-ascii collapses to separators
		"!!!":            "task",    // all punctuation falls back
		"--leading--":    "leading",
	}
	for in, want := range cases {
		if got := board.Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTaskFilename(t *testing.T) {
	if got := board.TaskFilename("task-12", "Add login"); got != "task-12 - Add-login.md" {
		t.Errorf("got %q", got)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	boardDir := newBoard(t)
	if _, err := board.WriteConfig(boardDir, board.Config{AutoCommit: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoCommit {
		t.Error("auto_commit not round-tripped")
	}
}

func TestLoadConfigTolerant(t *testing.T) {
	boardDir := newBoard(t)
	// Malformed YAML falls back to defaults instead of erroring.
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"), []byte("auto_commit: [oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatalf("should be tolerant: %v", err)
	}
	if cfg.AutoCommit {
		t.Error("expected default false on malformed config")
	}
}

func TestLoadConfigStringBool(t *testing.T) {
	boardDir := newBoard(t)
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"), []byte(`auto_commit: "true"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := board.LoadConfig(boardDir)
	if !cfg.AutoCommit {
		t.Error("string 'true' should parse as true")
	}
}

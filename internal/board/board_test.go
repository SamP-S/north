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
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md missing: %v", err)
	}
	for _, name := range append([]models.TaskStatus{}, models.StatusDirs...) {
		if fi, err := os.Stat(filepath.Join(boardDir, string(name))); err != nil || !fi.IsDir() {
			t.Errorf("status dir %s missing", name)
		}
	}
	if fi, err := os.Stat(filepath.Join(boardDir, "archive")); err != nil || !fi.IsDir() {
		t.Errorf("archive dir missing")
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPPort != 8001 || cfg.AutoCommit {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := board.InitBoard(root); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := board.InitBoard(root); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(agents)
	if string(data) != "custom" {
		t.Errorf("AGENTS.md overwritten: %q", data)
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

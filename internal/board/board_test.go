package board_test

import (
	"os"
	"path/filepath"
	"strings"
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
		"Add login form": "add-login-form",
		"  spaced  ":     "spaced",
		"a/b:c":          "a-b-c",
		"Café déjà":      "caf-d-j", // non-ascii collapses to separators
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
	if got := board.TaskFilename("12", "Add login"); got != "12-add-login.md" {
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

func TestLoadConfigRejectsMalformedYAML(t *testing.T) {
	boardDir := newBoard(t)
	// Malformed YAML must be a hard error, not a silent fall-back — a typo
	// would otherwise silently disable auto_commit.
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"), []byte("auto_commit: [oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := board.LoadConfig(boardDir)
	if be, ok := nerrors.As(err); !ok || be.Code() != "invalid" {
		t.Fatalf("expected invalid error, got %v", err)
	}
}

func TestInitScaffoldsBoardFiles(t *testing.T) {
	boardDir := newBoard(t)
	tmpl, err := os.ReadFile(filepath.Join(boardDir, board.TemplateName))
	if err != nil {
		t.Fatalf("task-template.md missing: %v", err)
	}
	if string(tmpl) != board.DefaultTaskTemplate {
		t.Errorf("template content: %q", tmpl)
	}
	ga, err := os.ReadFile(filepath.Join(boardDir, board.GitattributesName))
	if err != nil {
		t.Fatalf(".gitattributes missing: %v", err)
	}
	if string(ga) != "* text eol=lf\n" {
		t.Errorf(".gitattributes content: %q", ga)
	}
	// User edits to the template survive a re-init.
	custom := []byte("## Mine\n")
	if err := os.WriteFile(filepath.Join(boardDir, board.TemplateName), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := board.InitBoard(board.Root(boardDir)); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(boardDir, board.TemplateName))
	if string(got) != string(custom) {
		t.Error("re-init overwrote an edited template")
	}
}

func TestConfigVersionStamp(t *testing.T) {
	boardDir := newBoard(t)
	data, err := os.ReadFile(filepath.Join(boardDir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Errorf("init should stamp version: 1, got %q", data)
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != board.FormatVersion {
		t.Errorf("version = %d, want %d", cfg.Version, board.FormatVersion)
	}
	// A zero-version config write is normalised to the current format.
	if _, err := board.WriteConfig(boardDir, board.Config{AutoCommit: true}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = board.LoadConfig(boardDir)
	if cfg.Version != board.FormatVersion || !cfg.AutoCommit {
		t.Errorf("normalised write: %+v", cfg)
	}
}

func TestNewerVersionRefused(t *testing.T) {
	boardDir := newBoard(t)
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"),
		[]byte("version: 2\nauto_commit: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// LoadConfig refuses…
	_, err := board.LoadConfig(boardDir)
	if be, ok := nerrors.As(err); !ok || be.Code() != "conflict" {
		t.Fatalf("LoadConfig on newer board: %v", err)
	}
	// …and discovery refuses too, so every command is covered.
	_, err = board.LocateBoard(board.Root(boardDir))
	if be, ok := nerrors.As(err); !ok || be.Code() != "conflict" {
		t.Fatalf("LocateBoard on newer board: %v", err)
	}
}

func TestMissingVersionIsV1(t *testing.T) {
	boardDir := newBoard(t)
	// A pre-stamp board (no version key) loads as v1.
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"),
		[]byte("auto_commit: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || !cfg.AutoCommit {
		t.Errorf("pre-stamp board: %+v", cfg)
	}
	if _, err := board.LocateBoard(board.Root(boardDir)); err != nil {
		t.Errorf("pre-stamp board should still be discoverable: %v", err)
	}
}

func TestLoadConfigRejectsMalformedAutoCommit(t *testing.T) {
	boardDir := newBoard(t)
	// A typo like "flase" must be a hard error, not a silent false.
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"), []byte("auto_commit: flase\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := board.LoadConfig(boardDir)
	if be, ok := nerrors.As(err); !ok || be.Code() != "invalid" {
		t.Fatalf("expected invalid error, got %v", err)
	}
}

func TestSetConfigValuePreservesFile(t *testing.T) {
	boardDir := newBoard(t)
	content := "# my board notes\n" +
		"version: 1 # stamp\n" +
		"auto_commit: false\n" +
		"custom_key: kept\n" +
		"last_id: 41\n"
	if err := os.WriteFile(filepath.Join(boardDir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := board.SetConfigValue(boardDir, "max_wip", "3"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(boardDir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# my board notes", "# stamp", "custom_key: kept", "last_id: 41"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config set dropped %q: %q", want, data)
		}
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxWIP != 3 || cfg.LastID != 41 {
		t.Errorf("after set: max_wip=%d last_id=%d, want 3/41", cfg.MaxWIP, cfg.LastID)
	}
	// Typed values, not strings: the bool key round-trips as a YAML bool.
	if err := board.SetConfigValue(boardDir, "auto_commit", "true"); err != nil {
		t.Fatal(err)
	}
	cfg, err = board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoCommit || cfg.LastID != 41 {
		t.Errorf("after bool set: %+v", cfg)
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

// TestScaffoldedConfigMatchesDefaults guards the commented config.yml
// template against drifting from DefaultConfig: the file init writes must
// load back as exactly the defaults.
func TestScaffoldedConfigMatchesDefaults(t *testing.T) {
	boardDir := newBoard(t)
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != board.DefaultConfig() {
		t.Errorf("scaffolded config loads as %+v, defaults are %+v", cfg, board.DefaultConfig())
	}
	data, err := os.ReadFile(filepath.Join(boardDir, board.ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#") {
		t.Error("scaffolded config.yml carries no comments")
	}
}

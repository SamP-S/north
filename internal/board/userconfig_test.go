package board_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/board"
)

func TestLoadUserConfigMissingFile(t *testing.T) {
	cfg, err := board.LoadUserConfig(t.TempDir())
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if cfg.TUI.Theme != "default" {
		t.Errorf("theme = %q, want default", cfg.TUI.Theme)
	}
}

func TestEnsureUserConfigScaffolds(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".north")
	if err := board.EnsureUserConfig(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("scaffold missing: %v", err)
	}
	if !strings.Contains(string(data), "theme: default") {
		t.Errorf("scaffold should mention default theme, got:\n%s", data)
	}
	if !strings.Contains(string(data), "saturated") {
		t.Errorf("scaffold should document the theme options, got:\n%s", data)
	}
	cfg, err := board.LoadUserConfig(dir)
	if err != nil {
		t.Fatalf("scaffold should load cleanly: %v", err)
	}
	if cfg.TUI.Theme != "default" {
		t.Errorf("theme = %q, want default", cfg.TUI.Theme)
	}
}

func TestEnsureUserConfigPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	custom := "tui:\n  theme: saturated\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := board.EnsureUserConfig(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("existing file was rewritten: got %q, want %q", data, custom)
	}
}

func TestLoadUserConfigTheme(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("tui:\n  theme: saturated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := board.LoadUserConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUI.Theme != "saturated" {
		t.Errorf("theme = %q, want saturated", cfg.TUI.Theme)
	}
}

func TestLoadUserConfigUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("editor: vim\nfuture: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := board.LoadUserConfig(dir)
	if err != nil {
		t.Fatalf("unknown keys should be tolerated: %v", err)
	}
	if cfg.TUI.Theme != "default" {
		t.Errorf("theme = %q, want default", cfg.TUI.Theme)
	}
}

func TestLoadUserConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("tui: [not: closed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := board.LoadUserConfig(dir)
	if err == nil {
		t.Fatal("malformed YAML should return an error")
	}
	if cfg.TUI.Theme != "default" {
		t.Errorf("theme = %q, want default fallback", cfg.TUI.Theme)
	}
}

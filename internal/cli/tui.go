package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/tui"
)

// tuiOptions resolves the user-level config (~/.north/config.yml) into TUI
// options. Prefs must never block the TUI: any failure — no home dir,
// unwritable scaffold, unreadable file — falls back to the default theme,
// warning in the status bar where a warning is actionable.
func tuiOptions() tui.Options {
	dir, err := board.UserConfigDir()
	if err != nil {
		return tui.Options{}
	}
	path := filepath.Join(dir, board.ConfigName)
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = filepath.Join("~", rel)
		}
	}
	if err := board.EnsureUserConfig(dir); err != nil {
		return tui.Options{ThemeWarning: fmt.Sprintf("could not create %s (%v), using default theme", path, err)}
	}
	cfg, err := board.LoadUserConfig(dir)
	if err != nil {
		return tui.Options{ThemeWarning: fmt.Sprintf("could not read %s (%v), using default theme", path, err)}
	}
	return tui.Options{Theme: cfg.TUI.Theme, ConfigPath: path}
}

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			p := tea.NewProgram(
				tui.NewModel(boardDir, tuiOptions()),
				tea.WithAltScreen(),
			)
			_, err = p.Run()
			return err
		},
	}
}

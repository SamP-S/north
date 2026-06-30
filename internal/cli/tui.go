package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/tui"
)

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
				tui.NewModel(boardDir),
				tea.WithAltScreen(),
			)
			_, err = p.Run()
			return err
		},
	}
}

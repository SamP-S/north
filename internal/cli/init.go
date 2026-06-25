package cli

import (
	"github.com/SamP-S/north/internal/board"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "scaffold a north/ board in this repo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := board.InitBoard("")
			if err != nil {
				return err
			}
			cmd.Printf("Initialized north board at %s\n", dir)
			return nil
		},
	}
}

package cli

import (
	"fmt"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "scaffold a north/ board in this repo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Refuse when a board already exists here or in any ancestor —
			// a nested board would silently shadow discovery for this subtree.
			if existing, err := board.LocateBoard(""); err == nil {
				return nerrors.Conflict(fmt.Sprintf("north already initialised (%s)", existing))
			}
			dir, err := board.InitBoard("")
			if err != nil {
				return err
			}
			cmd.Printf("Initialized north board at %s\n", dir)
			return nil
		},
	}
}

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "scaffold a north/ board in this repo",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Refuse when a board already exists here or in any ancestor —
			// a nested board would silently shadow discovery for this subtree.
			existing, err := board.LocateBoard("")
			if err == nil {
				return nerrors.Conflict(fmt.Sprintf("north already initialised (%s)", existing))
			}
			// Only "no board found" clears the way; anything else (e.g. a
			// board created by a newer north) must not be shadowed either.
			if be, ok := nerrors.As(err); !ok || be.Code() != "not_found" {
				return err
			}
			dir, err := board.InitBoard("")
			if err != nil {
				return err
			}
			if asJSON {
				data, err := json.MarshalIndent(map[string]string{"board": dir}, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
				return nil
			}
			if plain {
				cmd.Println(dir)
				return nil
			}
			cmd.Printf("Initialized north board at %s\n", dir)
			cmd.Println()
			cmd.Println("Optional next steps:")
			cmd.Println("  north skill install                  teach AI agents this board (Claude Code + opencode)")
			cmd.Println("  north config set auto_commit true    local commit per board change (default: off)")
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

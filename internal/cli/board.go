package cli

import (
	"strings"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/render"
	"github.com/SamP-S/north/internal/tasks"
	"github.com/spf13/cobra"
)

func newBoardCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "board",
		Short: "board summary (counts per status)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			counts, err := tasks.StatusCounts(boardDir)
			if err != nil {
				return err
			}
			drafts, err := tasks.StateCount(boardDir, models.StateDraft)
			if err != nil {
				return err
			}
			archived, err := tasks.StateCount(boardDir, models.StateArchive)
			if err != nil {
				return err
			}
			out, err := render.Board(counts, drafts, archived, plain, asJSON)
			if err != nil {
				return err
			}
			cmd.Println(out)
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newCleanupCmd() *cobra.Command {
	var olderThan int
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "archive done tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			archived, err := tasks.Cleanup(boardDir, olderThan)
			if err != nil {
				return err
			}
			if plain || asJSON {
				out, err := render.TaskList(archived, plain, asJSON)
				if err != nil {
					return err
				}
				cmd.Println(out)
				return nil
			}
			if len(archived) == 0 {
				cmd.Println("Nothing to clean up.")
				return nil
			}
			ids := make([]string, len(archived))
			for i, t := range archived {
				ids[i] = t.ID
			}
			cmd.Printf("Archived %d done task(s): %s\n", len(archived), strings.Join(ids, ", "))
			return nil
		},
	}
	cmd.Flags().IntVar(&olderThan, "older-than", 0, "only archive done tasks older than DAYS")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

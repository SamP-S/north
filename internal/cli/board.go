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
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			snap, err := tasks.Load(boardDir)
			if err != nil {
				return err
			}
			if !asJSON {
				printWarnings(cmd, snap.Warnings)
			}
			out, err := render.Board(snap.StatusCounts(),
				snap.StateCount(models.StateDraft), snap.StateCount(models.StateArchive),
				snap.Warnings, plain, asJSON)
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
	var dryRun, plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "archive done tasks",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			archived, err := tasks.Cleanup(boardDir, olderThan, dryRun)
			if err != nil {
				return err
			}
			if plain || asJSON {
				out, err := render.CleanupReport(archived, dryRun, plain, asJSON)
				if err != nil {
					return err
				}
				if out != "" {
					cmd.Println(out)
				}
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
			if dryRun {
				cmd.Printf("Would archive %d done task(s): %s\n", len(archived), strings.Join(ids, ", "))
				return nil
			}
			cmd.Printf("Archived %d done task(s): %s\n", len(archived), strings.Join(ids, ", "))
			return nil
		},
	}
	cmd.Flags().IntVar(&olderThan, "older-than", 0, "only archive done tasks older than DAYS")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be archived without changing anything")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

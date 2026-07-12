package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/render"
	"github.com/SamP-S/north/internal/tasks"
	"github.com/spf13/cobra"
)

// printPickResult renders next/take output. "No workable task" is a normal
// outcome (exit 0): json emits an explicit {"task": null}, plain prints
// nothing, human prints a note. Snapshot warnings follow the list convention —
// stderr in human/plain, a "warnings" array in the json payload.
func printPickResult(cmd *cobra.Command, task *models.Task, warnings []tasks.Warning,
	plain, asJSON bool, humanLine func(*models.Task) string) error {
	if !asJSON {
		printWarnings(cmd, warnings)
	}
	switch {
	case asJSON:
		var m any
		if task != nil {
			m = task.ToMap()
		}
		warns := make([]string, len(warnings))
		for i, w := range warnings {
			warns[i] = w.String()
		}
		data, err := json.MarshalIndent(map[string]any{"task": m, "warnings": warns}, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
	case plain:
		if task == nil {
			return nil
		}
		out, err := render.TaskDetail(task, true, false)
		if err != nil {
			return err
		}
		cmd.Println(out)
	default:
		if task == nil {
			cmd.Println("No workable task.")
			return nil
		}
		cmd.Println(humanLine(task))
	}
	return nil
}

func newNextCmd() *cobra.Command {
	var plain, asJSON bool
	var labels []string
	cmd := &cobra.Command{
		Use:   "next",
		Short: "show the next workable task (read-only)",
		Long: "Show the next workable task: active, status ready, unassigned, all\n" +
			"dependencies met, lowest id first. A pure read — nothing is claimed.\n" +
			"No workable task is a normal outcome: exit 0 with {\"task\": null}\n" +
			"under --json, empty output under --plain.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, warnings, err := tasks.Next(boardDir, labels)
			if err != nil {
				return err
			}
			return printPickResult(cmd, task, warnings, plain, asJSON, func(t *models.Task) string {
				return fmt.Sprintf("Next: %s — %s", t.ID, t.Title)
			})
		},
	}
	cmd.Flags().StringSliceVar(&labels, "label", nil, "only consider tasks carrying this label (exact match; repeatable)")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newTakeCmd() *cobra.Command {
	var plain, asJSON bool
	var assignee string
	var labels []string
	cmd := &cobra.Command{
		Use:   "take",
		Short: "atomically claim the next workable task",
		Long: "Select the next workable task (same pick as `north next`) and claim it —\n" +
			"status in_progress plus assignee, in one write under the board lock — so\n" +
			"concurrent takes get different tasks. The assignee comes from --assignee,\n" +
			"falling back to the NORTH_AGENT environment variable. When the board's\n" +
			"max_wip is set (> 0), take refuses (conflict) while the assignee already\n" +
			"holds that many in_progress tasks. No workable task is a normal outcome:\n" +
			"exit 0 with {\"task\": null} under --json, empty output under --plain.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("assignee") {
				assignee = os.Getenv("NORTH_AGENT")
			}
			task, warnings, err := tasks.Take(boardDir, assignee, labels)
			if err != nil {
				return err
			}
			return printPickResult(cmd, task, warnings, plain, asJSON, func(t *models.Task) string {
				return fmt.Sprintf("Took %s (%s): %s", t.ID, t.Assignee, t.Title)
			})
		},
	}
	cmd.Flags().StringVar(&assignee, "assignee", "", "who claims the task (default: $NORTH_AGENT; required if unset)")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "only consider tasks carrying this label (exact match; repeatable)")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

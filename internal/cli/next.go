package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/render"
	"github.com/SamP-S/north/internal/tasks"
	"github.com/spf13/cobra"
)

// printPickResult renders next/take output. "No workable task" is a normal
// outcome (exit 0): json emits an explicit {"task": null}, plain prints
// nothing, human prints a note. Plain prints the task as a single list row
// (same columns as `task list --plain`). Snapshot warnings follow the list
// convention — stderr in human/plain, a "warnings" array in the json payload.
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
		out, err := render.TaskList([]*models.Task{task}, nil, true, false)
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
	var limit int
	cmd := &cobra.Command{
		Use:   "next",
		Short: "show the next workable task(s) (read-only)",
		Long: "Show the next workable task: active, status ready, unassigned, all\n" +
			"dependencies met, lowest id first. A pure read — nothing is claimed.\n" +
			"No workable task is a normal outcome: exit 0 with {\"task\": null}\n" +
			"under --json, empty output under --plain. With -l/--limit N (N ≥ 2,\n" +
			"or 0 = all) the next N tasks are shown in take order, rendered as a\n" +
			"task list ({\"tasks\": […]} under --json).",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return nerrors.Invalid(fmt.Sprintf("--limit must not be negative (got %d)", limit))
			}
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			picked, warnings, err := tasks.Next(boardDir, cleanLabelFilter(labels), limit)
			if err != nil {
				return err
			}
			if limit != 1 {
				return printPickList(cmd, picked, warnings, plain, asJSON)
			}
			var task *models.Task
			if len(picked) > 0 {
				task = picked[0]
			}
			return printPickResult(cmd, task, warnings, plain, asJSON, func(t *models.Task) string {
				return fmt.Sprintf("Next: %s — %s", t.ID, t.Title)
			})
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 1, "how many workable tasks to show, in take order (0 = all)")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "only consider tasks carrying this label (exact match; repeatable)")
	addOutputFlags(cmd, &plain, &asJSON)
	cmd.Flags().SetNormalizeFunc(aliasFlag("labels", "label"))
	return cmd
}

// printPickList renders next --limit N output as a task list. Snapshot
// warnings follow the list convention; an empty pick is exit 0 with
// {"tasks": []} under --json, no rows otherwise.
func printPickList(cmd *cobra.Command, picked []*models.Task, warnings []tasks.Warning, plain, asJSON bool) error {
	if !asJSON {
		printWarnings(cmd, warnings)
	}
	if asJSON {
		warns := make([]string, len(warnings))
		for i, w := range warnings {
			warns[i] = w.String()
		}
		data, err := json.MarshalIndent(map[string]any{"tasks": taskMaps(picked), "warnings": warns}, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}
	if len(picked) == 0 {
		// Plain prints nothing on an empty pick — no blank line.
		if !plain {
			cmd.Println("No workable task.")
		}
		return nil
	}
	out, err := render.TaskList(picked, warnings, plain, false)
	if err != nil {
		return err
	}
	cmd.Println(out)
	return nil
}

func newTakeCmd() *cobra.Command {
	var plain, asJSON bool
	var assignee string
	var labels []string
	cmd := &cobra.Command{
		Use:   "take [id]",
		Short: "atomically claim the next workable task (or a specific one)",
		Long: "Select the next workable task (same pick as `north next`) and claim it —\n" +
			"status in_progress plus assignee, in one write under the board lock — so\n" +
			"concurrent takes get different tasks. With an id, claim that specific task\n" +
			"instead: refused (conflict) unless it is active, ready, unassigned, and its\n" +
			"dependencies are met — no steal, no overrides. The assignee comes from\n" +
			"--assignee, falling back to the NORTH_AGENT environment variable. When the\n" +
			"board's max_wip is set (> 0), take refuses (conflict) while the assignee\n" +
			"already holds that many in_progress tasks. No workable task is a normal\n" +
			"outcome: exit 0 with {\"task\": null} under --json, empty output under\n" +
			"--plain.",
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			taskID := ""
			if len(args) == 1 {
				taskID = args[0]
				if len(labels) > 0 {
					return nerrors.Invalid("--label cannot be combined with an explicit task id")
				}
			}
			if !cmd.Flags().Changed("assignee") {
				assignee = os.Getenv("NORTH_AGENT")
			}
			task, warnings, err := tasks.Take(boardDir, assignee, taskID, cleanLabelFilter(labels))
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
	cmd.Flags().SetNormalizeFunc(aliasFlag("labels", "label"))
	return cmd
}

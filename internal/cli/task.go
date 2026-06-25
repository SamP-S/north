package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/render"
	"github.com/SamP-S/north/internal/tasks"
	"github.com/spf13/cobra"
)

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "manage tasks",
	}
	cmd.AddCommand(
		newTaskCreateCmd(),
		newTaskViewCmd(),
		newTaskListCmd(),
		newTaskEditCmd(),
		newTaskMoveCmd(),
		newTaskPromoteCmd(),
		newTaskDemoteCmd(),
		newTaskArchiveCmd(),
		newTaskRestoreCmd(),
		newTaskDeleteCmd(),
	)
	return cmd
}

// readBody returns the body from --body / --body-file, or nil if neither given.
func readBody(cmd *cobra.Command, body, bodyFile string) (*string, error) {
	if cmd.Flags().Changed("body-file") {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, nerrors.Invalid(fmt.Sprintf("body file not found: %s", bodyFile))
		}
		s := string(data)
		return &s, nil
	}
	if cmd.Flags().Changed("body") {
		return &body, nil
	}
	return nil, nil
}

func newTaskCreateCmd() *cobra.Command {
	var agent, body, bodyFile string
	var labels, dependsOn []string
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "create a task (lands in draft)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			bodyPtr, err := readBody(cmd, body, bodyFile)
			if err != nil {
				return err
			}
			bodyStr := ""
			if bodyPtr != nil {
				bodyStr = *bodyPtr
			}
			task, err := tasks.Create(boardDir, args[0], agent, labels, dependsOn, bodyStr)
			if err != nil {
				return err
			}
			cmd.Printf("Created %s (%s): %s\n", task.ID, task.State, task.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "executor/provider tag (opaque)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "free-form labels")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "task ids this depends on")
	cmd.Flags().StringVar(&body, "body", "", "task body text")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read task body from a file")
	return cmd
}

func newTaskViewCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "show a task (frontmatter + body)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, err := tasks.Get(boardDir, args[0])
			if err != nil {
				return err
			}
			out, err := render.TaskDetail(task, plain, asJSON)
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

func newTaskListCmd() *cobra.Command {
	var plain, asJSON bool
	var status, state string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list tasks (default: active)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			states, err := listStates(state)
			if err != nil {
				return err
			}
			ts, err := tasks.List(boardDir, states, status)
			if err != nil {
				return err
			}
			out, err := render.TaskList(ts, plain, asJSON)
			if err != nil {
				return err
			}
			cmd.Println(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&state, "state", "", "filter by state: draft|active|archive|all (default active)")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

// listStates maps the --state flag to the states to list (default: active).
func listStates(state string) ([]models.TaskState, error) {
	switch state {
	case "":
		return []models.TaskState{models.StateActive}, nil
	case "all":
		return models.StateOrder, nil
	default:
		s, err := tasks.ParseState(state)
		if err != nil {
			return nil, err
		}
		return []models.TaskState{s}, nil
	}
}

func newTaskEditCmd() *cobra.Command {
	var title, agent, body, bodyFile string
	var labels, dependsOn []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "edit a task's fields/body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			bodyPtr, err := readBody(cmd, body, bodyFile)
			if err != nil {
				return err
			}
			task, err := tasks.Edit(boardDir, args[0],
				changedString(cmd, "title", title),
				changedString(cmd, "agent", agent),
				changedSlice(cmd, "labels", labels),
				changedSlice(cmd, "depends-on", dependsOn),
				bodyPtr)
			if err != nil {
				return err
			}
			cmd.Printf("Edited %s\n", task.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "")
	cmd.Flags().StringVar(&agent, "agent", "", "")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "replace labels (empty to clear)")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "replace dependencies (empty to clear)")
	cmd.Flags().StringVar(&body, "body", "", "")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "")
	return cmd
}

func newTaskMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <id> <status>",
		Short: "change an active task's status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, err := tasks.SetStatus(boardDir, args[0], args[1])
			if err != nil {
				return err
			}
			cmd.Printf("%s → %s\n", task.ID, task.Status)
			return nil
		},
	}
}

// stateCmd builds a simple `task <verb> <id>` lifecycle command.
func stateCmd(use, short, doneWord string, op func(boardDir, id string) (*models.Task, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, err := op(boardDir, args[0])
			if err != nil {
				return err
			}
			cmd.Printf("%s %s (%s)\n", doneWord, task.ID, task.State)
			return nil
		},
	}
}

func newTaskPromoteCmd() *cobra.Command {
	return stateCmd("promote <id>", "promote a draft onto the active board", "Promoted", tasks.Promote)
}

func newTaskDemoteCmd() *cobra.Command {
	return stateCmd("demote <id>", "send an active task back to drafts", "Demoted", tasks.Demote)
}

func newTaskArchiveCmd() *cobra.Command {
	return stateCmd("archive <id>", "move a task to archive/", "Archived", tasks.Archive)
}

func newTaskRestoreCmd() *cobra.Command {
	return stateCmd("restore <id>", "restore an archived task to the active board", "Restored", tasks.Restore)
}

func newTaskDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "delete a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, err := tasks.Get(boardDir, args[0])
			if err != nil {
				return err
			}
			if !yes && !confirm(fmt.Sprintf("Delete %s (%s)?", task.ID, task.Title)) {
				cmd.Println("Aborted.")
				return errAborted
			}
			if err := tasks.Delete(boardDir, args[0]); err != nil {
				return err
			}
			cmd.Printf("Deleted %s\n", task.ID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

// errAborted yields a non-zero exit without printing an extra error line.
var errAborted = nerrors.Conflict("aborted")

func addOutputFlags(cmd *cobra.Command, plain, asJSON *bool) {
	cmd.Flags().BoolVar(plain, "plain", false, "stable unformatted output")
	cmd.Flags().BoolVar(asJSON, "json", false, "JSON output")
}

func changedString(cmd *cobra.Command, name, val string) *string {
	if cmd.Flags().Changed(name) {
		return &val
	}
	return nil
}

func changedSlice(cmd *cobra.Command, name string, val []string) *[]string {
	if cmd.Flags().Changed(name) {
		if val == nil {
			val = []string{}
		}
		return &val
	}
	return nil
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

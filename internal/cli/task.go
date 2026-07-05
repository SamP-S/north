package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
		newTaskStateCmd(),
		newTaskDeleteCmd(),
	)
	return cmd
}

// readBody returns the body from --body / --body-file, or nil if neither
// given. A body file of "-" reads stdin.
func readBody(cmd *cobra.Command, body, bodyFile string) (*string, error) {
	if cmd.Flags().Changed("body-file") {
		var data []byte
		var err error
		if bodyFile == "-" {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(bodyFile)
		}
		if err != nil {
			return nil, nerrors.Invalid(fmt.Sprintf("cannot read body file %s: %v", bodyFile, err))
		}
		s := string(data)
		return &s, nil
	}
	if cmd.Flags().Changed("body") {
		return &body, nil
	}
	return nil, nil
}

// printWarnings writes snapshot warnings to stderr (human and --plain modes;
// --json carries them in the payload instead).
func printWarnings(cmd *cobra.Command, warnings []tasks.Warning) {
	for _, w := range warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}
}

func newTaskCreateCmd() *cobra.Command {
	var agent, body, bodyFile string
	var labels, dependsOn []string
	var plain, asJSON bool
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
			if plain || asJSON {
				out, err := render.TaskDetail(task, plain, asJSON)
				if err != nil {
					return err
				}
				cmd.Println(out)
				return nil
			}
			cmd.Printf("Created %s (%s): %s\n", task.ID, task.State, task.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "executor/provider tag (opaque)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "free-form labels")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "task ids this depends on")
	cmd.Flags().StringVar(&body, "body", "", "task body text")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read task body from a file (- for stdin)")
	addOutputFlags(cmd, &plain, &asJSON)
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
	var status, state, search string
	var labels []string
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
			if status != "" {
				if _, err := tasks.ParseStatus(status); err != nil {
					return err
				}
			}
			snap, err := tasks.Load(boardDir)
			if err != nil {
				return err
			}
			ts := snap.Filter(states, status)
			ts = filterSearch(ts, search)
			ts = filterLabels(ts, labels)
			if !asJSON {
				printWarnings(cmd, snap.Warnings)
			}
			out, err := render.TaskList(ts, snap.Warnings, plain, asJSON)
			if err != nil {
				return err
			}
			cmd.Println(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&state, "state", "", "filter by state: draft|active|archive|all (default active)")
	cmd.Flags().StringVar(&search, "search", "", "filter by substring over id, title, agent, labels, and body (case-insensitive)")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "filter by label (exact match; repeatable)")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

// filterSearch keeps tasks whose id, title, agent, labels, or body contain q
// (case-insensitive). Empty q keeps everything.
func filterSearch(ts []*models.Task, q string) []*models.Task {
	if q == "" {
		return ts
	}
	q = strings.ToLower(q)
	out := make([]*models.Task, 0, len(ts))
	for _, t := range ts {
		if strings.Contains(strings.ToLower(t.ID), q) ||
			strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Agent), q) ||
			strings.Contains(strings.ToLower(t.Body), q) ||
			strings.Contains(strings.ToLower(strings.Join(t.Labels, "\n")), q) {
			out = append(out, t)
		}
	}
	return out
}

// filterLabels keeps tasks carrying every requested label (exact match).
func filterLabels(ts []*models.Task, labels []string) []*models.Task {
	if len(labels) == 0 {
		return ts
	}
	out := make([]*models.Task, 0, len(ts))
	for _, t := range ts {
		have := map[string]bool{}
		for _, l := range t.Labels {
			have[l] = true
		}
		all := true
		for _, l := range labels {
			if !have[l] {
				all = false
				break
			}
		}
		if all {
			out = append(out, t)
		}
	}
	return out
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
	var title, agent, body, bodyFile, appendBody string
	var labels, dependsOn []string
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "edit a task's fields/body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("append-body") &&
				(cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file")) {
				return nerrors.Invalid("--append-body cannot be combined with --body/--body-file")
			}
			bodyPtr, err := readBody(cmd, body, bodyFile)
			if err != nil {
				return err
			}
			opts := tasks.EditOpts{
				Title:     changedString(cmd, "title", title),
				Agent:     changedString(cmd, "agent", agent),
				Labels:    changedSlice(cmd, "labels", labels),
				DependsOn: changedSlice(cmd, "depends-on", dependsOn),
				Body:      bodyPtr,
			}
			if cmd.Flags().Changed("append-body") {
				opts.AppendBody = &appendBody
			}
			task, err := tasks.Edit(boardDir, args[0], opts)
			if err != nil {
				return err
			}
			if plain || asJSON {
				out, err := render.TaskDetail(task, plain, asJSON)
				if err != nil {
					return err
				}
				cmd.Println(out)
				return nil
			}
			cmd.Printf("Edited %s\n", task.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&agent, "agent", "", "executor or provider tag")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "replace labels (empty to clear)")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "replace dependencies (empty to clear)")
	cmd.Flags().StringVar(&body, "body", "", "replace body text")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "replace body from a file (- for stdin)")
	cmd.Flags().StringVar(&appendBody, "append-body", "", "append text to the body (blank-line separated)")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newTaskMoveCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "move <id> <status>",
		Short: "set a task's status (any status → any status, any state)",
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
			if task.State != models.StateActive {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: %s is %s; status shows on the board once active\n",
					task.ID, task.State)
			}
			if plain || asJSON {
				out, err := render.TaskDetail(task, plain, asJSON)
				if err != nil {
					return err
				}
				cmd.Println(out)
				return nil
			}
			cmd.Printf("%s → %s\n", task.ID, task.Status)
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newTaskStateCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "state <id> <draft|active|archive>",
		Short: "set a task's lifecycle state (any state → any state)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, err := tasks.SetState(boardDir, args[0], args[1])
			if err != nil {
				return err
			}
			if plain || asJSON {
				out, err := render.TaskDetail(task, plain, asJSON)
				if err != nil {
					return err
				}
				cmd.Println(out)
				return nil
			}
			cmd.Printf("%s state → %s\n", task.ID, task.State)
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newTaskDeleteCmd() *cobra.Command {
	var yes, plain, asJSON bool
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
			dependents, err := tasks.Dependents(boardDir, task.ID)
			if err != nil {
				return err
			}
			depIDs := make([]string, len(dependents))
			for i, d := range dependents {
				depIDs[i] = d.ID
			}
			if len(depIDs) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s depend on %s\n", strings.Join(depIDs, ", "), task.ID)
			}
			if !yes {
				// Machine modes and non-TTY stdin never prompt: require -y.
				if plain || asJSON || !stdinIsTTY(cmd) {
					return nerrors.Invalid("delete requires -y when not run interactively")
				}
				if !confirm(cmd, fmt.Sprintf("Delete %s (%s)?", task.ID, task.Title)) {
					fmt.Fprintln(cmd.ErrOrStderr(), "Aborted.")
					return errAborted
				}
			}
			if err := tasks.Delete(boardDir, args[0]); err != nil {
				return err
			}
			if asJSON {
				m := task.ToMap()
				m["warnings"] = depIDs
				data, err := json.MarshalIndent(m, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
				return nil
			}
			if plain {
				out, err := render.TaskDetail(task, true, false)
				if err != nil {
					return err
				}
				cmd.Println(out)
				return nil
			}
			cmd.Printf("Deleted %s\n", task.ID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	addOutputFlags(cmd, &plain, &asJSON)
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

// stdinIsTTY reports whether the command's stdin is an interactive terminal.
// Non-file readers (tests injecting input) count as interactive.
func stdinIsTTY(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return true
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// confirm prompts on stderr and reads a y/N answer from stdin.
func confirm(cmd *cobra.Command, prompt string) bool {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

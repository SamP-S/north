package cli

import (
	"bufio"
	"encoding/json"
	"errors"
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
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// aliasFlag returns a flag-normalize func mapping only alias to canonical,
// leaving every other flag name untouched. Used to forgive --label/--labels
// mix-ups: the flag stays registered (and documented) under its canonical
// name, so help output is unchanged — only lookups of the alias are rewritten.
func aliasFlag(alias, canonical string) func(*pflag.FlagSet, string) pflag.NormalizedName {
	return func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == alias {
			name = canonical
		}
		return pflag.NormalizedName(name)
	}
}

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

// printTaskResult renders a mutated task in machine modes, returning true
// when it printed. Plain prints the task as a single list row — the same
// shape as batches and `task list --plain` (`view` alone shows the detail
// record). Advisory op warnings go to stderr in human/plain modes; the JSON
// payload is {"task": {…}, "warnings": […]} — the same wrapper next/take use,
// warnings always an array, never null.
func printTaskResult(cmd *cobra.Command, task *models.Task, warns []string, plain, asJSON bool) (bool, error) {
	if !asJSON {
		for _, w := range warns {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
		}
	}
	switch {
	case asJSON:
		if warns == nil {
			warns = []string{}
		}
		out := map[string]any{"task": task.ToMap(), "warnings": warns}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return false, err
		}
		cmd.Println(string(data))
		return true, nil
	case plain:
		out, err := render.TaskList([]*models.Task{task}, nil, true, false)
		if err != nil {
			return false, err
		}
		cmd.Println(out)
		return true, nil
	}
	return false, nil
}

// splitIDs parses a comma-delimited id argument, trimming and deduplicating
// while preserving order.
func splitIDs(arg string) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	for _, id := range strings.Split(arg, ",") {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nerrors.Invalid("no task ids given")
	}
	return ids, nil
}

// batchErr is one failed id in a batch operation.
type batchErr struct {
	ID  string
	Err error
}

// errCode returns an error's board-error code ("internal" for plain errors).
func errCode(err error) string {
	if be, ok := nerrors.As(err); ok {
		return be.Code()
	}
	return "internal"
}

// batchReport renders a batch's per-id results and returns the summarising
// error for the exit-code contract: nil when everything succeeded, otherwise
// an error carrying the shared failure code (internal when the codes differ).
// warns are advisory op warnings — stderr in human/plain, a "warnings" key
// in the JSON payload.
func batchReport(cmd *cobra.Command, done []*models.Task, errs []batchErr, warns []string,
	plain, asJSON bool, line func(*models.Task) string) error {
	switch {
	case asJSON:
		if warns == nil {
			warns = []string{}
		}
		out := map[string]any{"tasks": taskMaps(done), "errors": errMaps(errs), "warnings": warns}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
	case plain:
		out, err := render.TaskList(done, nil, true, false)
		if err != nil {
			return err
		}
		if out != "" {
			cmd.Println(out)
		}
	default:
		for _, t := range done {
			cmd.Println(line(t))
		}
	}
	if !asJSON {
		for _, w := range warns {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
		}
		for _, e := range errs {
			fmt.Fprintf(cmd.ErrOrStderr(), "error [%s]: %s\n", errCode(e.Err), e.Err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	shared := errCode(errs[0].Err)
	for _, e := range errs[1:] {
		if errCode(e.Err) != shared {
			shared = "internal"
			break
		}
	}
	msg := fmt.Sprintf("%d of %d ids failed", len(errs), len(errs)+len(done))
	switch shared {
	case "invalid":
		return nerrors.Invalid(msg)
	case "not_found":
		return nerrors.NotFound(msg)
	case "conflict":
		return nerrors.Conflict(msg)
	default:
		return fmt.Errorf("%s", msg)
	}
}

func taskMaps(ts []*models.Task) []map[string]any {
	out := make([]map[string]any, len(ts))
	for i, t := range ts {
		out[i] = t.ToMap()
	}
	return out
}

func errMaps(errs []batchErr) []map[string]string {
	out := make([]map[string]string, len(errs))
	for i, e := range errs {
		out[i] = map[string]string{"id": e.ID, "code": errCode(e.Err), "message": e.Err.Error()}
	}
	return out
}

func newTaskCreateCmd() *cobra.Command {
	var assignee, body, bodyFile string
	var labels, dependsOn []string
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "create a task (lands in draft)",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			bodyPtr, err := readBody(cmd, body, bodyFile)
			if err != nil {
				return err
			}
			// Bodyless creates fill from the board's task-template.md.
			bodyStr := tasks.TemplateBody(boardDir)
			if bodyPtr != nil {
				bodyStr = *bodyPtr
			}
			task, warns, err := tasks.Create(boardDir, args[0], assignee, labels, dependsOn, bodyStr)
			if err != nil {
				return err
			}
			if printed, err := printTaskResult(cmd, task, warns, plain, asJSON); err != nil || printed {
				return err
			}
			cmd.Printf("Created %s (%s): %s\n", task.ID, task.State, task.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&assignee, "assignee", "", "who works this task — a person or an agent (free-form)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "free-form labels")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "task ids this depends on")
	cmd.Flags().StringVar(&body, "body", "", "task body text")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read task body from a file (- for stdin)")
	addOutputFlags(cmd, &plain, &asJSON)
	cmd.Flags().SetNormalizeFunc(aliasFlag("label", "labels"))
	return cmd
}

func newTaskViewCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "show a task (frontmatter + body)",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			task, err := tasks.Get(boardDir, args[0])
			if err != nil {
				return err
			}
			// --json wraps as {"task": …, "warnings": []} — the same envelope
			// every mutation payload uses.
			if asJSON {
				_, err := printTaskResult(cmd, task, nil, false, true)
				return err
			}
			out, err := render.TaskDetail(task, plain)
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
	var plain, asJSON, reverse bool
	var status, state, search, sortBy, assignee, deps string
	var labels []string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list tasks (default: active)",
		Args:  noArgs,
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
			key, err := tasks.ParseSortKey(sortBy)
			if err != nil {
				return err
			}
			ts := snap.Filter(states, status)
			ts = filterSearch(ts, search)
			ts = filterLabels(ts, cleanLabelFilter(labels))
			if cmd.Flags().Changed("assignee") {
				ts = filterAssignee(ts, assignee)
			}
			switch deps {
			case "":
			case "met", "unmet":
				wantUnmet := deps == "unmet"
				kept := make([]*models.Task, 0, len(ts))
				for _, t := range ts {
					if (len(snap.UnmetDeps(t)) > 0) == wantUnmet {
						kept = append(kept, t)
					}
				}
				ts = kept
			default:
				return nerrors.Invalid(fmt.Sprintf("unknown --deps %q (expected met or unmet)", deps))
			}
			// Natural directions: newest first for id/updated, A→Z for
			// title/assignee; --reverse flips.
			desc := key == tasks.SortID || key == tasks.SortUpdated
			if reverse {
				desc = !desc
			}
			tasks.Sort(ts, key, desc)
			if limit < 0 {
				return nerrors.Invalid(fmt.Sprintf("--limit must not be negative (got %d)", limit))
			}
			if limit > 0 && len(ts) > limit {
				ts = ts[:limit]
			}
			if !asJSON {
				printWarnings(cmd, snap.Warnings)
			}
			out, err := render.TaskList(ts, snap.Warnings, plain, asJSON)
			if err != nil {
				return err
			}
			// An empty plain list renders as "" — print nothing rather than a
			// blank line (matches empty `next --plain`).
			if out != "" {
				cmd.Println(out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&state, "state", "", "filter by state: draft|active|archive|all (default active)")
	cmd.Flags().StringVar(&search, "search", "", "filter by substring over id, title, assignee, labels, and body (case-insensitive)")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "filter by label (exact match; repeatable)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "filter by assignee (case-insensitive; empty matches unassigned)")
	cmd.Flags().StringVar(&deps, "deps", "", "filter by dependency resolution: met|unmet (a dep resolves when done or archived)")
	cmd.Flags().StringVar(&sortBy, "sort", "id", "sort by: id|updated|title|assignee (id/updated newest-first, title/assignee A→Z)")
	cmd.Flags().BoolVar(&reverse, "reverse", false, "reverse the sort direction")
	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "show at most N tasks after filtering and sorting (0 = unlimited)")
	addOutputFlags(cmd, &plain, &asJSON)
	cmd.Flags().SetNormalizeFunc(aliasFlag("labels", "label"))
	return cmd
}

// filterSearch keeps tasks matching q per tasks.MatchesSearch (id, title,
// assignee, labels, body; case-insensitive). Empty q keeps everything.
func filterSearch(ts []*models.Task, q string) []*models.Task {
	if q == "" {
		return ts
	}
	out := make([]*models.Task, 0, len(ts))
	for _, t := range ts {
		if tasks.MatchesSearch(t, q) {
			out = append(out, t)
		}
	}
	return out
}

// filterAssignee keeps tasks whose assignee matches case-insensitively
// ("" matches unassigned tasks).
func filterAssignee(ts []*models.Task, assignee string) []*models.Task {
	out := make([]*models.Task, 0, len(ts))
	for _, t := range ts {
		if strings.EqualFold(t.Assignee, assignee) {
			out = append(out, t)
		}
	}
	return out
}

// cleanLabelFilter trims --label values and drops empties, so filters agree
// with the trimmed labels tasks store.
func cleanLabelFilter(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// filterLabels keeps tasks carrying every requested label (exact match),
// sharing the matcher next/take select with.
func filterLabels(ts []*models.Task, labels []string) []*models.Task {
	if len(labels) == 0 {
		return ts
	}
	out := make([]*models.Task, 0, len(ts))
	for _, t := range ts {
		if tasks.HasLabels(t, labels) {
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
	var title, assignee, body, bodyFile, appendBody string
	var labels, dependsOn []string
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "edit a task's fields/body",
		Args:  exactArgs(1),
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
				Assignee:  changedString(cmd, "assignee", assignee),
				Labels:    changedSlice(cmd, "labels", labels),
				DependsOn: changedSlice(cmd, "depends-on", dependsOn),
				Body:      bodyPtr,
			}
			if cmd.Flags().Changed("append-body") {
				opts.AppendBody = &appendBody
			}
			task, warns, err := tasks.Edit(boardDir, args[0], opts)
			if err != nil {
				return err
			}
			if printed, err := printTaskResult(cmd, task, warns, plain, asJSON); err != nil || printed {
				return err
			}
			cmd.Printf("Edited %s\n", task.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&assignee, "assignee", "", "who works this task — a person or an agent (free-form)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "replace labels (empty to clear)")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "replace dependencies (empty to clear)")
	cmd.Flags().StringVar(&body, "body", "", "replace body text")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "replace body from a file (- for stdin)")
	cmd.Flags().StringVar(&appendBody, "append-body", "", "append text to the body (blank-line separated)")
	addOutputFlags(cmd, &plain, &asJSON)
	cmd.Flags().SetNormalizeFunc(aliasFlag("label", "labels"))
	return cmd
}

func newTaskMoveCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "move <id[,id…]> <status>",
		Short: "set task status (any status → any status, any state)",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			ids, err := splitIDs(args[0])
			if err != nil {
				return err
			}
			if len(ids) == 1 {
				task, warns, err := tasks.SetStatus(boardDir, ids[0], args[1])
				if err != nil {
					return err
				}
				if printed, err := printTaskResult(cmd, task, warns, plain, asJSON); err != nil || printed {
					return err
				}
				cmd.Printf("%s → %s\n", task.ID, task.Status)
				return nil
			}
			var done []*models.Task
			var errs []batchErr
			var warnsAll []string
			for _, id := range ids {
				task, warns, err := tasks.SetStatus(boardDir, id, args[1])
				if err != nil {
					errs = append(errs, batchErr{ID: id, Err: err})
					continue
				}
				warnsAll = append(warnsAll, warns...)
				done = append(done, task)
			}
			return batchReport(cmd, done, errs, warnsAll, plain, asJSON, func(t *models.Task) string {
				return fmt.Sprintf("%s → %s", t.ID, t.Status)
			})
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newTaskStateCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "state <id[,id…]> <draft|active|archive>",
		Short: "set task lifecycle state (any state → any state)",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			ids, err := splitIDs(args[0])
			if err != nil {
				return err
			}
			if len(ids) == 1 {
				task, warns, err := tasks.SetState(boardDir, ids[0], args[1])
				if err != nil {
					return err
				}
				if printed, err := printTaskResult(cmd, task, warns, plain, asJSON); err != nil || printed {
					return err
				}
				cmd.Printf("%s state → %s\n", task.ID, task.State)
				return nil
			}
			var done []*models.Task
			var errs []batchErr
			var warnsAll []string
			for _, id := range ids {
				task, warns, err := tasks.SetState(boardDir, id, args[1])
				if err != nil {
					errs = append(errs, batchErr{ID: id, Err: err})
					continue
				}
				warnsAll = append(warnsAll, warns...)
				done = append(done, task)
			}
			return batchReport(cmd, done, errs, warnsAll, plain, asJSON, func(t *models.Task) string {
				return fmt.Sprintf("%s state → %s", t.ID, t.State)
			})
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newTaskDeleteCmd() *cobra.Command {
	var yes, plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "delete <id[,id…]>",
		Short: "delete tasks",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			ids, err := splitIDs(args[0])
			if err != nil {
				return err
			}
			if len(ids) > 1 {
				if !yes {
					return nerrors.Invalid("deleting multiple tasks requires -y")
				}
				var done []*models.Task
				var errs []batchErr
				var warnsAll []string
				for _, id := range ids {
					task, warns, err := tasks.Delete(boardDir, id)
					if err != nil {
						errs = append(errs, batchErr{ID: id, Err: err})
						continue
					}
					warnsAll = append(warnsAll, warns...)
					done = append(done, task)
				}
				return batchReport(cmd, done, errs, warnsAll, plain, asJSON, func(t *models.Task) string {
					return fmt.Sprintf("Deleted %s", t.ID)
				})
			}
			if !yes {
				// Machine modes and non-TTY stdin never prompt: require -y.
				if plain || asJSON || !stdinIsTTY(cmd) {
					return nerrors.Invalid("delete requires -y when not run interactively")
				}
				// The prompt needs the title up front; only this interactive
				// path pre-reads the task.
				preview, err := tasks.Get(boardDir, ids[0])
				if err != nil {
					return err
				}
				// Surface dependents before the decision, not after.
				warnDependents(cmd, boardDir, preview)
				if !confirm(cmd, fmt.Sprintf("Delete %s (%s)?", preview.ID, preview.Title)) {
					fmt.Fprintln(cmd.ErrOrStderr(), "Aborted.")
					return errAborted
				}
			}
			task, warns, err := tasks.Delete(boardDir, ids[0])
			if err != nil {
				return err
			}
			if printed, err := printTaskResult(cmd, task, warns, plain, asJSON); err != nil || printed {
				return err
			}
			cmd.Printf("Deleted %s\n", task.ID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

// warnDependents prints a stderr warning when other tasks depend on task.
func warnDependents(cmd *cobra.Command, boardDir string, task *models.Task) {
	dependents, err := tasks.Dependents(boardDir, task.ID)
	if err != nil {
		return
	}
	depIDs := make([]string, len(dependents))
	for i, d := range dependents {
		depIDs[i] = d.ID
	}
	if len(depIDs) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s depend on %s\n", strings.Join(depIDs, ", "), task.ID)
	}
}

// errAborted yields a non-zero exit (1, the internal fallback — a user abort
// is no domain conflict) without printing an extra error line.
var errAborted = errors.New("aborted")

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
// Non-file readers (tests injecting input) count as interactive. A char-device
// check is not enough here: /dev/null is a char device but not a terminal.
func stdinIsTTY(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return true
	}
	return term.IsTerminal(int(f.Fd()))
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

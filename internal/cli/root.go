// Package cli defines the `north` command tree and dispatches each subcommand.
//
// Commands operate directly on the in-repo north/ board (discovered by walking
// up from cwd) — there is no service to reach. Failures print
// "error [<code>]: <msg>" to stderr; when the failing command was invoked with
// --json, the error is emitted as a JSON object instead so agents can parse
// it. Exit codes follow one contract in every output mode: 0 success,
// 1 internal (or user-aborted), 2 invalid/usage, 3 not_found, 4 conflict.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/version"
	"github.com/spf13/cobra"
)

// Execute builds the root command and runs it, returning a process exit code.
func Execute() int {
	root := newRootCmd()
	cmd, err := root.ExecuteC()
	if err != nil {
		if err == errAborted {
			// User-facing message already printed; exit with the abort code
			// (1, the internal fallback) without an extra error line.
			return nerrors.ExitCode(err)
		}
		// Cobra reports an unmatched subcommand as a plain error; it is a
		// usage mistake under the exit-code contract. The string-prefix check
		// is coupled to cobra's error text by design: a cobra upgrade that
		// rewords it flips the exit code from 2 to 1, and the CLI exit-code
		// test pins the wording so the break is loud in CI, not silent in
		// the field.
		if _, ok := nerrors.As(err); !ok && strings.HasPrefix(err.Error(), "unknown command") {
			err = nerrors.Invalid(err.Error())
		}
		code := "internal"
		msg := err.Error()
		if be, ok := nerrors.As(err); ok {
			code = be.Code()
			msg = be.Message()
		}
		if jsonRequested(cmd) {
			payload, merr := json.Marshal(map[string]any{
				"error": map[string]string{"code": code, "message": msg},
			})
			if merr == nil {
				fmt.Fprintln(os.Stderr, string(payload))
				return nerrors.ExitCode(err)
			}
		}
		fmt.Fprintf(os.Stderr, "error [%s]: %s\n", code, msg)
		return nerrors.ExitCode(err)
	}
	return 0
}

// exactArgs and noArgs wrap cobra's validators so argument-count mistakes are
// usage errors (exit 2) under the exit-code contract.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return nerrors.Invalid(err.Error())
		}
		return nil
	}
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return nerrors.Invalid(err.Error())
	}
	return nil
}

func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(n)(cmd, args); err != nil {
			return nerrors.Invalid(err.Error())
		}
		return nil
	}
}

// jsonRequested reports whether the executed command was invoked with --json.
func jsonRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	f := cmd.Flags().Lookup("json")
	return f != nil && f.Changed && f.Value.String() == "true"
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "north",
		Short:         "North task board CLI",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Cobra's Print helpers fall back to stderr when no out-writer is set;
	// command output must land on stdout so `north … --json | jq` works.
	root.SetOut(os.Stdout)
	// Flag mistakes are usage errors (exit 2), not internal failures.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return nerrors.Invalid(err.Error())
	})
	root.SetVersionTemplate("north {{.Version}}\n")
	root.AddCommand(
		newInitCmd(),
		newTaskCmd(),
		newNextCmd(),
		newTakeCmd(),
		newBoardCmd(),
		newCleanupCmd(),
		newDoctorCmd(),
		newConfigCmd(),
		newSkillCmd(),
		newTuiCmd(),
		newVersionCmd(),
	)
	return root
}

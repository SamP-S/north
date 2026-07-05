// Package cli defines the `north` command tree and dispatches each subcommand.
//
// Commands operate directly on the in-repo north/ board (discovered by walking
// up from cwd) — there is no service to reach. Failures print "error: <msg>"
// to stderr and exit 1; when the failing command was invoked with --json, the
// error is emitted as a JSON object instead so agents can parse it.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

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
			return 1 // user-facing message already printed
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
				return 1
			}
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
		return 1
	}
	return 0
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
	root.SetVersionTemplate("north {{.Version}}\n")
	root.AddCommand(
		newInitCmd(),
		newTaskCmd(),
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

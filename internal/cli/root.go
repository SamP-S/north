// Package cli defines the `north` command tree and dispatches each subcommand.
//
// Commands operate directly on the in-repo north/ board (discovered by walking
// up from cwd) — there is no service to reach. BoardError failures print to
// stderr and exit 1.
package cli

import (
	"fmt"
	"os"

	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/spf13/cobra"
)

// Execute builds the root command and runs it, returning a process exit code.
func Execute() int {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		if err == errAborted {
			return 1 // user-facing message already printed
		}
		if be, ok := nerrors.As(err); ok {
			fmt.Fprintf(os.Stderr, "error: %s\n", be.Message())
		} else {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "north",
		Short:         "North task board CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newInitCmd(),
		newTaskCmd(),
		newBoardCmd(),
		newCleanupCmd(),
		newSkillCmd(),
	)
	return root
}

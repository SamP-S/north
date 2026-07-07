package cli

import (
	"github.com/SamP-S/north/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the north version",
		Args:  noArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println("north " + version.Version)
		},
	}
}

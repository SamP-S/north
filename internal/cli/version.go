package cli

import (
	"encoding/json"

	"github.com/SamP-S/north/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "print the north version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case asJSON:
				data, err := json.MarshalIndent(map[string]string{"version": version.Version}, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
			case plain:
				cmd.Println(version.Version)
			default:
				cmd.Println("north " + version.Version)
			}
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

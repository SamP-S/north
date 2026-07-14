package cli

import (
	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/render"
	"github.com/SamP-S/north/internal/tasks"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var fix, plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "check the board for integrity problems",
		Long: "Scan every task file for problems: unparseable files, duplicate ids,\n" +
			"filename/frontmatter id drift, dangling depends_on references, dependency\n" +
			"cycles, and CRLF line endings. With --fix, safe repairs are applied\n" +
			"(CRLF rewrite, duplicate renumbering, filename renames).",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			issues, err := tasks.Doctor(boardDir, fix)
			if err != nil {
				return err
			}
			out, err := render.DoctorReport(issues, plain, asJSON)
			if err != nil {
				return err
			}
			cmd.Println(out)
			// Issues found are the report, not a command failure: a completed
			// scan exits 0 whatever it found. Gate on the --json issues array.
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "repair what is safe to repair")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

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
			"cycles, unparseable timestamps, and CRLF line endings; a missing\n" +
			"north/.gitattributes or north/.gitignore is reported too.\n\n" +
			"With --fix, safe repairs are applied: CRLF files are rewritten with LF,\n" +
			"duplicate ids are renumbered (the first holder keeps the id), drifted\n" +
			"filenames are renamed to match their frontmatter id, dangling depends_on\n" +
			"references are removed, and a missing .gitattributes/.gitignore is\n" +
			"restored. Unparseable files, cycles, unparseable timestamps, and deps\n" +
			"pointing at an unparseable file (the dep may be valid once that file is\n" +
			"repaired) are reported but never changed. With auto_commit on, all\n" +
			"repairs land in a single commit (\"north: doctor fixes\").",
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
			// scan always exits 0. Scripts that need to act on findings must
			// inspect the --json issues array, not the exit code.
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "repair what is safe to repair")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

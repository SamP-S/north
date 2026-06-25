package cli

import (
	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/skill"
	"github.com/spf13/cobra"
)

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "manage the north agent skill",
	}
	cmd.AddCommand(newSkillInstallCmd(), newSkillShowCmd())
	return cmd
}

func newSkillInstallCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install the north skill for Claude Code and opencode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if !global {
				// Anchor project installs at the repo root (the board's parent)
				// when a board exists, else the current directory.
				if b, err := board.LocateBoard(""); err == nil {
					root = board.Root(b)
				}
			}
			targets, err := skill.Install(root, global)
			if err != nil {
				return err
			}
			for _, t := range targets {
				cmd.Printf("Installed %s skill → %s\n", t.Agent, t.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "install into the home-dir skill locations instead of the project")
	return cmd
}

func newSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "print the embedded skill",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Print(skill.Content())
			return nil
		},
	}
}

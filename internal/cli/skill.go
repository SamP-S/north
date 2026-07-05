package cli

import (
	"fmt"
	"os"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/skill"
	"github.com/spf13/cobra"
)

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "manage the north agent skill",
	}
	cmd.AddCommand(newSkillInstallCmd(), newSkillShowCmd(), newSkillCheckCmd())
	return cmd
}

// skillRoot resolves where project skill installs are anchored: the repo root
// when a board exists, else the current directory.
func skillRoot() string {
	if b, err := board.LocateBoard(""); err == nil {
		return board.Root(b)
	}
	return "."
}

func newSkillCheckCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "report whether installed skills match this binary's version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			targets, err := skill.Targets(skillRoot(), global)
			if err != nil {
				return err
			}
			stale := 0
			for _, t := range targets {
				data, err := os.ReadFile(t.Path)
				if err != nil {
					cmd.Printf("%s: missing (%s) — run `north skill install`\n", t.Agent, t.Path)
					stale++
					continue
				}
				installed := skill.InstalledVersion(string(data))
				if installed != skill.Version {
					cmd.Printf("%s: outdated (installed %s, binary %s) — run `north skill install`\n",
						t.Agent, installed, skill.Version)
					stale++
					continue
				}
				cmd.Printf("%s: up to date (%s)\n", t.Agent, installed)
			}
			if stale > 0 {
				return nerrors.Conflict(fmt.Sprintf("%d skill install(s) missing or outdated", stale))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "check the home-dir skill locations instead of the project")
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

package cli

import (
	"encoding/json"
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

// checkTarget is one row of `skill check` output: the install state of one
// agent's skill location.
type checkTarget struct {
	Agent     string `json:"agent"`
	Path      string `json:"path"`
	Installed string `json:"installed"` // "" when missing or unstamped
	Binary    string `json:"binary"`
	Status    string `json:"status"` // ok|outdated|missing
}

func newSkillCheckCmd() *cobra.Command {
	var global, plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "report whether installed skills match this binary's version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			targets, err := skill.Targets(skillRoot(), global)
			if err != nil {
				return err
			}
			results := make([]checkTarget, 0, len(targets))
			outdated, missing := 0, 0
			for _, t := range targets {
				ct := checkTarget{Agent: t.Agent, Path: t.Path, Binary: skill.Version}
				data, err := os.ReadFile(t.Path)
				if err != nil {
					ct.Status = "missing"
					missing++
				} else if ct.Installed = skill.InstalledVersion(string(data)); ct.Installed != skill.Version {
					ct.Status = "outdated"
					outdated++
				} else {
					ct.Status = "ok"
				}
				results = append(results, ct)
			}
			switch {
			case asJSON:
				data, err := json.MarshalIndent(map[string]any{"targets": results}, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
			case plain:
				for _, ct := range results {
					cmd.Printf("%s\t%s\t%s\t%s\n", ct.Agent, ct.Status, ct.Installed, ct.Path)
				}
			default:
				for _, ct := range results {
					switch ct.Status {
					case "missing":
						// Missing is only a warning — installs may be selective
						// (`install --target`), so absence is not an error.
						cmd.Printf("%s: not installed (%s)\n", ct.Agent, ct.Path)
					case "outdated":
						cmd.Printf("%s: outdated (installed %s, binary %s) — run `north skill install`\n",
							ct.Agent, ct.Installed, ct.Binary)
					default:
						cmd.Printf("%s: up to date (%s)\n", ct.Agent, ct.Installed)
					}
				}
			}
			if outdated > 0 {
				return nerrors.Conflict(fmt.Sprintf("%d skill install(s) outdated", outdated))
			}
			if missing == len(targets) {
				return nerrors.NotFound("skill not installed anywhere — run `north skill install`")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "check the home-dir skill locations instead of the project")
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newSkillInstallCmd() *cobra.Command {
	var global bool
	var targetNames []string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install the north skill for Claude Code and/or opencode",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if !global {
				// Anchor project installs at the repo root (the board's parent)
				// when a board exists, else the current directory.
				if b, err := board.LocateBoard(""); err == nil {
					root = board.Root(b)
				}
			}
			// Unknown --target names arrive already typed Invalid (exit 2);
			// I/O failures stay internal (exit 1).
			targets, err := skill.Install(root, global, targetNames...)
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
	cmd.Flags().StringSliceVar(&targetNames, "target", nil, "install only for these tools: claude|opencode (repeatable; default both)")
	return cmd
}

func newSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "print the embedded skill",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Print(skill.Content())
			return nil
		},
	}
}

package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// Agent describes an AI coding agent and where its skills live.
type Agent struct {
	Name        string // identifier
	DisplayName string // human-readable
	ProjectDir  string // skill base dir relative to the repo root
	GlobalDir   string // skill base dir relative to the user's home dir
}

// agents is the registry of supported agents. opencode also natively reads
// .claude/skills, but we install to its own dir too for explicitness.
var agents = []Agent{
	{Name: "claude", DisplayName: "Claude Code", ProjectDir: ".claude/skills", GlobalDir: ".claude/skills"},
	{Name: "opencode", DisplayName: "opencode", ProjectDir: ".opencode/skills", GlobalDir: ".config/opencode/skills"},
}

// Agents returns the supported agents.
func Agents() []Agent { return agents }

// Target is a resolved install location for one agent.
type Target struct {
	Agent string // agent display name
	Dir   string // absolute path to the skill dir (…/north)
	Path  string // absolute path to the SKILL.md file
}

// Targets resolves install locations for every agent. When global is true they
// resolve under the user's home dir, otherwise under projectRoot.
func Targets(projectRoot string, global bool) ([]Target, error) {
	var home string
	if global {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = h
	}
	var targets []Target
	for _, a := range agents {
		base := a.ProjectDir
		root := projectRoot
		if global {
			base = a.GlobalDir
			root = home
		}
		dir := filepath.Join(root, base, Name)
		targets = append(targets, Target{
			Agent: a.DisplayName,
			Dir:   dir,
			Path:  filepath.Join(dir, "SKILL.md"),
		})
	}
	return targets, nil
}

// Install writes the embedded SKILL.md into every agent's skill dir (under
// projectRoot, or the home dir when global). Returns the written targets.
func Install(projectRoot string, global bool) ([]Target, error) {
	targets, err := Targets(projectRoot, global)
	if err != nil {
		return nil, err
	}
	content := []byte(Content())
	for _, t := range targets {
		if err := os.MkdirAll(t.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating skill dir %s: %w", t.Dir, err)
		}
		if err := os.WriteFile(t.Path, content, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", t.Path, err)
		}
	}
	return targets, nil
}

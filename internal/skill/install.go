package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Target is a resolved install location for one agent.
type Target struct {
	Agent string // agent display name
	Dir   string // absolute path to the skill dir (…/north)
	Path  string // absolute path to the SKILL.md file
}

// Targets resolves install locations for the named agents (all when names is
// empty). When global is true they resolve under the user's home dir,
// otherwise under projectRoot. Unknown names are an error.
func Targets(projectRoot string, global bool, names ...string) ([]Target, error) {
	selected, err := selectAgents(names)
	if err != nil {
		return nil, err
	}
	var home string
	if global {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = h
	}
	var targets []Target
	for _, a := range selected {
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

// selectAgents maps names to registry entries; empty names selects all.
func selectAgents(names []string) ([]Agent, error) {
	if len(names) == 0 {
		return agents, nil
	}
	var out []Agent
	for _, n := range names {
		found := false
		for _, a := range agents {
			if a.Name == n {
				out = append(out, a)
				found = true
				break
			}
		}
		if !found {
			known := make([]string, len(agents))
			for i, a := range agents {
				known[i] = a.Name
			}
			return nil, fmt.Errorf("unknown skill target %q (expected one of: %s)",
				n, strings.Join(known, ", "))
		}
	}
	return out, nil
}

// Install writes the embedded SKILL.md into the named agents' skill dirs (all
// agents when names is empty), under projectRoot or the home dir when global.
// Returns the written targets.
func Install(projectRoot string, global bool, names ...string) ([]Target, error) {
	targets, err := Targets(projectRoot, global, names...)
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

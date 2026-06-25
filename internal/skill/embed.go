// Package skill embeds the north agent skill and installs it into the skill
// directories of AI coding agents (Claude Code, opencode).
package skill

import (
	_ "embed"
	"strings"
)

// Name is the skill's directory name and identifier.
const Name = "north"

// Version is the embedded skill's version, stamped into installed SKILL.md
// files so `north skill` can detect outdated installs.
const Version = "0.1.0"

const versionPrefix = "<!-- north-skill-version: "
const versionSuffix = " -->"

//go:embed skill/SKILL.md
var skillMD string

// Content returns the SKILL.md text with the version comment injected after the
// frontmatter block.
func Content() string {
	return injectVersion(skillMD, Version)
}

// injectVersion inserts the version comment after the closing "---" of the
// frontmatter; if there is no frontmatter, it prepends the comment.
func injectVersion(content, ver string) string {
	comment := versionPrefix + ver + versionSuffix
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return comment + "\n" + content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, comment)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return comment + "\n" + content
}

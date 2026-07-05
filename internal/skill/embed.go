// Package skill embeds the north agent skill and installs it into the skill
// directories of AI coding agents (Claude Code, opencode).
package skill

import (
	_ "embed"
	"strings"

	"github.com/SamP-S/north/internal/version"
)

// Name is the skill's directory name and identifier.
const Name = "north"

// Version is the embedded skill's version, stamped into installed SKILL.md
// files so `north skill` can detect outdated installs.
var Version = version.Version

const versionPrefix = "<!-- north-skill-version: "
const versionSuffix = " -->"

//go:embed skill/SKILL.md
var skillMD string

// Content returns the SKILL.md text with the version comment injected after the
// frontmatter block.
func Content() string {
	return injectVersion(skillMD, Version)
}

// InstalledVersion extracts the version stamp from an installed SKILL.md's
// content ("" when no stamp is present).
func InstalledVersion(content string) string {
	i := strings.Index(content, versionPrefix)
	if i < 0 {
		return ""
	}
	rest := content[i+len(versionPrefix):]
	j := strings.Index(rest, versionSuffix)
	if j < 0 {
		return ""
	}
	return rest[:j]
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

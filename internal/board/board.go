// Package board handles board discovery, scaffolding, and config.
//
// The board is a "north/" directory inside the user's project repo, anchored by
// "north/config.yml". It is found by walking up from the current directory, the
// same way git finds ".git". Tasks live in one of three state folders:
// drafts/, tasks/, archive/.
package board

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
	"gopkg.in/yaml.v3"
)

const (
	BoardDirname = "north"
	ConfigName   = "config.yml"

	// TemplateName is the editable body scaffold used by bodyless creates.
	TemplateName = "task-template.md"
	// GitattributesName guards board files against CRLF drift on any clone.
	GitattributesName = ".gitattributes"
	// GitignoreName keeps transient board files (.lock, *.tmp) out of git.
	GitignoreName = ".gitignore"

	// FormatVersion is the board format this binary reads and writes. Boards
	// stamped with a newer version are refused on load.
	FormatVersion = 1
)

// DefaultTaskTemplate is what init scaffolds into north/task-template.md. It
// is a suggestion, never a schema: North writes it once and never parses it
// back, and a missing or empty template means new tasks get a blank body —
// this constant is not a runtime fallback.
const DefaultTaskTemplate = `## Summary

What this task is and why it exists, in a sentence or two.

## Acceptance Criteria

Checklist of verifiable outcomes that mean "done".

## Notes

Context, links, and constraints useful to whoever picks this up.

## Changes

Running log of what was done, dated as work happens.

## Comments

Discussion and review remarks from humans or agents.
`

const gitattributesContent = "* text eol=lf\n"

const gitignoreContent = ".lock\n*.tmp\n"

// defaultConfigContent is the commented config.yml init scaffolds — the
// valid values live in the file itself (the same discoverability story as
// the user-level TUI config). Comments survive until the first
// `north config set`, which rewrites the file plain; the header says so.
// Values must match DefaultConfig.
const defaultConfigContent = `# north board settings — read/write with ` + "`north config get|set|list`" + `
# (note: ` + "`north config set`" + ` rewrites this file without comments)
version: 1                   # board format stamp (read-only)
auto_commit: false           # commit each board change locally (never pushes/pulls)
deps_enforcement: validated  # depends_on enforcement: hint | validated | strict
max_wip: 0                   # per-assignee in_progress cap enforced by ` + "`north take`" + ` (0 = unlimited)
`

var (
	idRe       = regexp.MustCompile(`^(\d+)-`)
	taskFileRe = regexp.MustCompile(`^\d+-.*\.md$`)
	nonAlnumRe = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// DepsEnforcement grades how strictly depends_on is enforced on writes.
// Enforcement never touches stored data, so levels switch freely.
type DepsEnforcement string

const (
	// DepsHint never refuses a dependency-related action; it only warns
	// (dangling/forward references, self-refs, cycles, out-of-order moves).
	DepsHint DepsEnforcement = "hint"
	// DepsValidated keeps the graph well-formed — dangling ids, self-refs,
	// and cycles are refused on write, delete heals dependents — while
	// workflow order (finishing with unmet deps) only warns. The default.
	DepsValidated DepsEnforcement = "validated"
	// DepsStrict is validated plus workflow enforcement: moving to done or
	// in_progress with unmet dependencies is refused.
	DepsStrict DepsEnforcement = "strict"
)

// DepsEnforcements lists the levels, loosest first.
var DepsEnforcements = []DepsEnforcement{DepsHint, DepsValidated, DepsStrict}

// ParseDepsEnforcement coerces a string to a DepsEnforcement level.
func ParseDepsEnforcement(value string) (DepsEnforcement, error) {
	for _, l := range DepsEnforcements {
		if DepsEnforcement(value) == l {
			return l, nil
		}
	}
	return "", errors.Invalid(fmt.Sprintf(
		"unknown deps_enforcement %q (expected one of: hint, validated, strict)", value))
}

// Config holds per-board settings from "north/config.yml".
type Config struct {
	Version         int             `yaml:"version"`
	AutoCommit      bool            `yaml:"auto_commit"`
	DepsEnforcement DepsEnforcement `yaml:"deps_enforcement"`
	// MaxWIP caps how many active in_progress tasks one assignee may hold,
	// enforced only by `north take` (0 = unlimited).
	MaxWIP int `yaml:"max_wip"`
	// LastID is the id high-water mark: the largest id ever allocated, so
	// deleting the newest task can never hand its id to the next create.
	// Managed by AllocateID (read-only via `config set`); 0 on boards from
	// before the mark existed, where the file scan alone decides.
	LastID int `yaml:"last_id"`
}

// DefaultConfig returns the defaults (current format version, auto_commit
// false, deps validated, max_wip unlimited).
func DefaultConfig() Config {
	return Config{Version: FormatVersion, AutoCommit: false, DepsEnforcement: DepsValidated, MaxWIP: 0}
}

// LocateBoard walks up from start (default cwd) to find the "north/" board dir
// (the one containing config.yml). Returns NotFound with a hint if none exists.
func LocateBoard(start string) (string, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = cwd
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(current, BoardDirname, ConfigName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if err := checkVersion(candidate); err != nil {
				return "", err
			}
			return filepath.Join(current, BoardDirname), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", errors.NotFound("no north board found (run `north init` in your project repo)")
}

// Root returns the project repo root that contains the board (north/ parent).
func Root(board string) string {
	return filepath.Dir(board)
}

// StateDir returns the absolute folder path for a state within the board.
func StateDir(board string, state models.TaskState) string {
	return filepath.Join(board, models.StateDirs[state])
}

// InitBoard scaffolds a board under root (default cwd): config + state folders.
// Idempotent — existing files/folders are left untouched. Returns the board dir.
func InitBoard(root string) (string, error) {
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = cwd
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	board := filepath.Join(root, BoardDirname)
	if err := os.MkdirAll(board, 0o755); err != nil {
		return "", err
	}
	for _, state := range models.StateOrder {
		if err := os.MkdirAll(StateDir(board, state), 0o755); err != nil {
			return "", err
		}
	}
	configPath := filepath.Join(board, ConfigName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(defaultConfigContent), 0o644); err != nil {
			return "", err
		}
	}
	templatePath := filepath.Join(board, TemplateName)
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		if err := os.WriteFile(templatePath, []byte(DefaultTaskTemplate), 0o644); err != nil {
			return "", err
		}
	}
	gaPath := filepath.Join(board, GitattributesName)
	if _, err := os.Stat(gaPath); os.IsNotExist(err) {
		if err := WriteGitattributes(board); err != nil {
			return "", err
		}
	}
	giPath := filepath.Join(board, GitignoreName)
	if _, err := os.Stat(giPath); os.IsNotExist(err) {
		if err := WriteGitignore(board); err != nil {
			return "", err
		}
	}
	return board, nil
}

// WriteGitignore writes the board's .gitignore (".lock", "*.tmp"),
// overwriting. Used by init and `doctor --fix`.
func WriteGitignore(board string) error {
	return os.WriteFile(filepath.Join(board, GitignoreName), []byte(gitignoreContent), 0o644)
}

// WriteGitattributes writes the board's .gitattributes ("* text eol=lf"),
// overwriting. Used by init and `doctor --fix`.
func WriteGitattributes(board string) error {
	return os.WriteFile(filepath.Join(board, GitattributesName), []byte(gitattributesContent), 0o644)
}

// checkVersion refuses a config.yml stamped with a newer format version.
// Unreadable or malformed files are tolerated here — LoadConfig raises those
// where config values actually matter.
func checkVersion(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if yaml.Unmarshal(data, &raw) != nil {
		return nil
	}
	if v := toInt(raw["version"], FormatVersion); v > FormatVersion {
		return errors.Conflict(fmt.Sprintf(
			"board format version %d was created by a newer north (this binary supports version %d) — upgrade north", v, FormatVersion))
	}
	return nil
}

// LoadConfig reads "north/config.yml" into a Config. A missing file and extra
// keys are tolerated; malformed YAML is a hard error (a silently ignored typo
// would silently change behaviour, e.g. turning auto_commit off).
func LoadConfig(board string) (Config, error) {
	cfg := DefaultConfig()
	path := filepath.Join(board, ConfigName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return cfg, errors.Invalid(fmt.Sprintf("malformed %s: %v", path, err))
	}
	if v, ok := raw["auto_commit"]; ok {
		cfg.AutoCommit = toBool(v, cfg.AutoCommit)
	}
	if v, ok := raw["deps_enforcement"]; ok {
		s, _ := v.(string)
		level, err := ParseDepsEnforcement(s)
		if err != nil {
			return cfg, errors.Invalid(fmt.Sprintf("%s: %v", path, err))
		}
		cfg.DepsEnforcement = level
	}
	if v, ok := raw["max_wip"]; ok {
		n := toInt(v, -1)
		if n < 0 {
			return cfg, errors.Invalid(fmt.Sprintf(
				"%s: max_wip must be a non-negative integer (got %v)", path, v))
		}
		cfg.MaxWIP = n
	}
	if v, ok := raw["last_id"]; ok {
		if n := toInt(v, 0); n > 0 {
			cfg.LastID = n
		}
	}
	// A missing version key means a board from before the stamp existed — v1.
	cfg.Version = toInt(raw["version"], FormatVersion)
	if cfg.Version > FormatVersion {
		return cfg, errors.Conflict(fmt.Sprintf(
			"board format version %d was created by a newer north (this binary supports version %d) — upgrade north", cfg.Version, FormatVersion))
	}
	return cfg, nil
}

// WriteConfig writes config.yml, stamping the current format version when the
// config carries none. Returns the path.
func WriteConfig(board string, cfg Config) (string, error) {
	if cfg.Version == 0 {
		cfg.Version = FormatVersion
	}
	if cfg.DepsEnforcement == "" {
		cfg.DepsEnforcement = DepsValidated
	}
	path := filepath.Join(board, ConfigName)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// TaskFiles returns the task Markdown files in the given state folders (all
// states if none are given), sorted within each folder.
func TaskFiles(board string, states ...models.TaskState) ([]string, error) {
	if len(states) == 0 {
		states = models.StateOrder
	}
	var files []string
	for _, state := range states {
		dir := StateDir(board, state)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var matched []string
		for _, e := range entries {
			if !e.IsDir() && taskFileRe.MatchString(e.Name()) {
				matched = append(matched, filepath.Join(dir, e.Name()))
			}
		}
		sort.Strings(matched)
		files = append(files, matched...)
	}
	return files, nil
}

// NextID returns the next free task id — a bare number, one past the highest
// of the on-disk scan and the config's last_id high-water mark. The scan alone
// would reuse the id of a deleted newest task; the mark closes that hole.
func NextID(board string) (string, error) {
	files, err := TaskFiles(board)
	if err != nil {
		return "", err
	}
	highest := 0
	for _, path := range files {
		m := idRe.FindStringSubmatch(filepath.Base(path))
		if m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > highest {
				highest = n
			}
		}
	}
	cfg, err := LoadConfig(board)
	if err != nil {
		return "", err
	}
	if cfg.LastID > highest {
		highest = cfg.LastID
	}
	return strconv.Itoa(highest + 1), nil
}

// AllocateID hands out the next free task id and persists it as the config's
// last_id high-water mark, so the id can never be reissued after a delete.
// The caller must hold the board lock (allocation is a read-bump-write).
func AllocateID(board string) (string, error) {
	id, err := NextID(board)
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return "", err
	}
	if err := setConfigLastID(board, n); err != nil {
		return "", err
	}
	return id, nil
}

// setConfigLastID rewrites config.yml with last_id set to n, going through a
// yaml.Node round-trip so user comments, key order, and unknown keys survive
// (unlike WriteConfig, which re-marshals the struct plain).
func setConfigLastID(board string, n int) error {
	path := filepath.Join(board, ConfigName)
	var doc yaml.Node
	if data, err := os.ReadFile(path); err == nil {
		// Malformed YAML falls through to a fresh mapping: LoadConfig has
		// already rejected genuinely malformed boards before allocation.
		_ = yaml.Unmarshal(data, &doc)
	}
	m := configMapping(&doc)
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
	set := false
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "last_id" {
			// Keep the key node (and any comments on it); swap the value only.
			val.LineComment = m.Content[i+1].LineComment
			m.Content[i+1] = val
			set = true
			break
		}
	}
	if !set {
		m.Content = append(m.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "last_id"}, val)
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

// configMapping returns the top-level mapping of a parsed config document,
// or a fresh empty mapping when the document is empty or not a mapping.
func configMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		return doc.Content[0]
	}
	return &yaml.Node{Kind: yaml.MappingNode}
}

// Slug builds a filename-safe slug from a title (Backlog.md-style, dash-separated).
func Slug(title string) string {
	cleaned := strings.Trim(nonAlnumRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if cleaned == "" {
		return "task"
	}
	return cleaned
}

// TaskFilename is the on-disk filename for a task: "12-add-login.md".
func TaskFilename(taskID, title string) string {
	return fmt.Sprintf("%s-%s.md", taskID, Slug(title))
}

func toInt(v any, fallback int) int {
	switch n := v.(type) {
	case int:
		return n
	case string:
		if p, err := strconv.Atoi(n); err == nil {
			return p
		}
	}
	return fallback
}

func toBool(v any, fallback bool) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		if pb, err := strconv.ParseBool(b); err == nil {
			return pb
		}
	}
	return fallback
}

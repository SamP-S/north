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

	// FormatVersion is the board format this binary reads and writes. Boards
	// stamped with a newer version are refused on load.
	FormatVersion = 1
)

// DefaultTaskTemplate is what init scaffolds into north/task-template.md. It
// is a suggestion, never a schema: North writes it once and never parses it
// back, and a missing or empty template means new tasks get a blank body —
// this constant is not a runtime fallback.
const DefaultTaskTemplate = `## Summary

## Acceptance Criteria

## Notes

## Changes

## Comments
`

const gitattributesContent = "* text eol=lf\n"

var (
	idRe       = regexp.MustCompile(`^(\d+)-`)
	taskFileRe = regexp.MustCompile(`^\d+-.*\.md$`)
	nonAlnumRe = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// Config holds per-board settings from "north/config.yml".
type Config struct {
	Version    int  `yaml:"version"`
	AutoCommit bool `yaml:"auto_commit"`
}

// DefaultConfig returns the MVP defaults (current format version, auto_commit false).
func DefaultConfig() Config {
	return Config{Version: FormatVersion, AutoCommit: false}
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
		if _, err := WriteConfig(board, DefaultConfig()); err != nil {
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
	return board, nil
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

// NextID returns the next free task id — a bare number, max across all
// folders + 1 (as a string; ids are never reused).
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
	return strconv.Itoa(highest + 1), nil
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

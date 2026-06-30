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
)

var (
	idRe       = regexp.MustCompile(`^task-(\d+)`)
	nonAlnumRe = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// Config holds per-board settings from "north/config.yml".
type Config struct {
	AutoCommit bool `yaml:"auto_commit"`
}

// DefaultConfig returns the MVP defaults (auto_commit false).
func DefaultConfig() Config {
	return Config{AutoCommit: false}
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
	return board, nil
}

// LoadConfig reads "north/config.yml" into a Config (tolerant of missing file
// and extra keys).
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
		return cfg, nil // tolerant: fall back to defaults on malformed config
	}
	if v, ok := raw["auto_commit"]; ok {
		cfg.AutoCommit = toBool(v, cfg.AutoCommit)
	}
	return cfg, nil
}

// WriteConfig writes config.yml. Returns the path.
func WriteConfig(board string, cfg Config) (string, error) {
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
			if !e.IsDir() && strings.HasPrefix(e.Name(), "task-") && strings.HasSuffix(e.Name(), ".md") {
				matched = append(matched, filepath.Join(dir, e.Name()))
			}
		}
		sort.Strings(matched)
		files = append(files, matched...)
	}
	return files, nil
}

// NextID returns the next free "task-<n>" id (max across all folders + 1).
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
	return fmt.Sprintf("task-%d", highest+1), nil
}

// Slug builds a filename-safe slug from a title (Backlog.md-style, dash-separated).
func Slug(title string) string {
	cleaned := strings.Trim(nonAlnumRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if cleaned == "" {
		return "task"
	}
	return cleaned
}

// TaskFilename is the on-disk filename for a task: "task-12-add-login.md".
func TaskFilename(taskID, title string) string {
	return fmt.Sprintf("%s-%s.md", taskID, Slug(title))
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

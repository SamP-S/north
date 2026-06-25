// Package board handles board discovery, scaffolding, and config.
//
// The board is a "north/" directory inside the user's project repo, anchored by
// "north/config.yml". It is found by walking up from the current directory, the
// same way git finds ".git".
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
	"github.com/SamP-S/north/internal/instructions"
	"github.com/SamP-S/north/internal/models"
	"gopkg.in/yaml.v3"
)

const (
	BoardDirname = "north"
	ConfigName   = "config.yml"
	ArchiveDir   = "archive"
	AgentsFile   = "AGENTS.md"
)

var (
	idRe       = regexp.MustCompile(`^task-(\d+)`)
	nonAlnumRe = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// Config holds per-board settings from "north/config.yml".
type Config struct {
	MCPPort    int  `yaml:"mcp_port"`
	AutoCommit bool `yaml:"auto_commit"`
}

// DefaultConfig returns the MVP defaults (mcp_port 8001, auto_commit false).
func DefaultConfig() Config {
	return Config{MCPPort: 8001, AutoCommit: false}
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

// InitBoard scaffolds a board under root (default cwd): config, folders,
// AGENTS.md. Idempotent — existing files/folders are left untouched. Returns the
// board dir.
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
	dirs := append([]string{}, statusDirNames()...)
	dirs = append(dirs, ArchiveDir)
	for _, name := range dirs {
		if err := os.MkdirAll(filepath.Join(board, name), 0o755); err != nil {
			return "", err
		}
	}
	configPath := filepath.Join(board, ConfigName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, err := WriteConfig(board, DefaultConfig()); err != nil {
			return "", err
		}
	}
	agentsPath := filepath.Join(root, AgentsFile)
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		if err := os.WriteFile(agentsPath, []byte(instructions.AgentsMD()), 0o644); err != nil {
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
	if v, ok := raw["mcp_port"]; ok {
		cfg.MCPPort = toInt(v, cfg.MCPPort)
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

// TaskFiles returns all task Markdown files across status folders (and
// optionally archive), sorted within each folder.
func TaskFiles(board string, includeArchive bool) ([]string, error) {
	dirs := append([]string{}, statusDirNames()...)
	if includeArchive {
		dirs = append(dirs, ArchiveDir)
	}
	var files []string
	for _, name := range dirs {
		dir := filepath.Join(board, name)
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
	files, err := TaskFiles(board, true)
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
	cleaned := strings.Trim(nonAlnumRe.ReplaceAllString(title, "-"), "-")
	if cleaned == "" {
		return "task"
	}
	return cleaned
}

// TaskFilename is the on-disk filename for a task: "task-12 - Add-login.md".
func TaskFilename(taskID, title string) string {
	return fmt.Sprintf("%s - %s.md", taskID, Slug(title))
}

func statusDirNames() []string {
	names := make([]string, len(models.StatusDirs))
	for i, s := range models.StatusDirs {
		names[i] = string(s)
	}
	return names
}

func toInt(v any, fallback int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
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

package board

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SamP-S/north/internal/errors"
	"gopkg.in/yaml.v3"
)

// userConfigScaffold is what EnsureUserConfig writes on first run. The
// comments are the discoverability story: users find the valid values in
// the file itself.
const userConfigScaffold = `# north user settings (per-user, not per-board)
tui:
  # theme: default | saturated | high-contrast
  theme: default
`

// UserConfig holds user-level (per-user, not per-board) settings read from
// "~/.north/config.yml". Unlike Config, it is personal preference and never
// committed to the shared repo.
type UserConfig struct {
	TUI struct {
		Theme string `yaml:"theme"`
	} `yaml:"tui"`
}

// DefaultUserConfig returns the defaults (theme "default").
func DefaultUserConfig() UserConfig {
	var cfg UserConfig
	cfg.TUI.Theme = "default"
	return cfg
}

// UserConfigDir returns the user-level config directory, "~/.north".
func UserConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".north"), nil
}

// LoadUserConfig reads dir/config.yml into a UserConfig. A missing file and
// unknown keys are tolerated; a read error or malformed YAML returns the
// defaults alongside the error so callers can fall back and warn — they
// never block on this file.
func LoadUserConfig(dir string) (UserConfig, error) {
	cfg := DefaultUserConfig()
	path := filepath.Join(dir, ConfigName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return DefaultUserConfig(), errors.Invalid(fmt.Sprintf("malformed %s: %v", path, err))
	}
	if cfg.TUI.Theme == "" {
		cfg.TUI.Theme = "default"
	}
	return cfg, nil
}

// EnsureUserConfig creates dir and scaffolds dir/config.yml with the
// commented template if the file does not exist. An existing file is left
// untouched.
func EnsureUserConfig(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, ConfigName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(userConfigScaffold), 0o644)
}

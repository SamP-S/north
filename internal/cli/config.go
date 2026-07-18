package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/spf13/cobra"
)

// configKeys lists the config keys visible to list/get. version (the board's
// format stamp) and last_id (the id high-water mark, managed by allocation)
// are read-only; set refuses them.
var configKeys = []string{"version", "auto_commit", "deps_enforcement", "max_wip", "last_id"}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "read and write north/config.yml settings",
	}
	cmd.AddCommand(newConfigListCmd(), newConfigGetCmd(), newConfigSetCmd())
	return cmd
}

func loadBoardConfig() (string, board.Config, error) {
	boardDir, err := board.LocateBoard("")
	if err != nil {
		return "", board.Config{}, err
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return "", board.Config{}, err
	}
	return boardDir, cfg, nil
}

// configTypedValue returns one config value with its real type (int, bool,
// or string) for JSON payloads.
func configTypedValue(cfg board.Config, key string) (any, error) {
	switch key {
	case "version":
		return cfg.Version, nil
	case "auto_commit":
		return cfg.AutoCommit, nil
	case "deps_enforcement":
		return string(cfg.DepsEnforcement), nil
	case "max_wip":
		return cfg.MaxWIP, nil
	case "last_id":
		return cfg.LastID, nil
	default:
		return nil, nerrors.Invalid(fmt.Sprintf("unknown config key %q (known: %s)", key, joinKeys()))
	}
}

func configValue(cfg board.Config, key string) (string, error) {
	v, err := configTypedValue(cfg, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", v), nil
}

func joinKeys() string { return strings.Join(configKeys, ", ") }

// printKeyValueJSON prints the {"key": …, "value": …} payload shared by
// config get and set, value typed.
func printKeyValueJSON(cmd *cobra.Command, key string, value any) error {
	data, err := json.MarshalIndent(map[string]any{"key": key, "value": value}, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(string(data))
	return nil
}

func newConfigListCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "print every config key and value",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadBoardConfig()
			if err != nil {
				return err
			}
			if asJSON {
				// Field order matches configKeys; typed values, not strings.
				payload := struct {
					Version         int    `json:"version"`
					AutoCommit      bool   `json:"auto_commit"`
					DepsEnforcement string `json:"deps_enforcement"`
					MaxWIP          int    `json:"max_wip"`
					LastID          int    `json:"last_id"`
				}{cfg.Version, cfg.AutoCommit, string(cfg.DepsEnforcement), cfg.MaxWIP, cfg.LastID}
				data, err := json.MarshalIndent(map[string]any{"config": payload}, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
				return nil
			}
			for _, key := range configKeys {
				v, err := configValue(cfg, key)
				if err != nil {
					return err
				}
				if plain {
					cmd.Printf("%s\t%s\n", key, v)
				} else {
					cmd.Printf("%s: %s\n", key, v)
				}
			}
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "print one config value",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadBoardConfig()
			if err != nil {
				return err
			}
			v, err := configTypedValue(cfg, args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return printKeyValueJSON(cmd, args[0], v)
			}
			// Human and plain agree here: the bare value alone is stable.
			cmd.Println(fmt.Sprintf("%v", v))
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	var plain, asJSON bool
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "set one config value",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, err := board.LocateBoard("")
			if err != nil {
				return err
			}
			key, value := args[0], args[1]
			unlock, err := board.Lock(boardDir)
			if err != nil {
				return err
			}
			defer unlock()
			// Re-read under the lock so a concurrent writer's changes (e.g. a
			// last_id bump from task creation) are seen, never overwritten.
			if _, err := board.LoadConfig(boardDir); err != nil {
				return err
			}
			var canonical string
			switch key {
			case "version":
				return nerrors.Invalid("version is read-only (the board's format stamp)")
			case "last_id":
				return nerrors.Invalid("last_id is read-only (the id high-water mark, managed by task creation)")
			case "auto_commit":
				b, err := strconv.ParseBool(value)
				if err != nil {
					return nerrors.Invalid(fmt.Sprintf("auto_commit must be true or false (got %q)", value))
				}
				canonical = strconv.FormatBool(b)
			case "deps_enforcement":
				level, err := board.ParseDepsEnforcement(value)
				if err != nil {
					return err
				}
				canonical = string(level)
			case "max_wip":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					return nerrors.Invalid(fmt.Sprintf(
						"max_wip must be a non-negative integer, 0 = unlimited (got %q)", value))
				}
				canonical = strconv.Itoa(n)
			default:
				return nerrors.Invalid(fmt.Sprintf("unknown config key %q (known: %s)", key, joinKeys()))
			}
			if err := board.SetConfigValue(boardDir, key, canonical); err != nil {
				return err
			}
			if asJSON || plain {
				cfg, err := board.LoadConfig(boardDir)
				if err != nil {
					return err
				}
				v, err := configTypedValue(cfg, key)
				if err != nil {
					return err
				}
				if asJSON {
					return printKeyValueJSON(cmd, key, v)
				}
				cmd.Printf("%s\t%v\n", key, v)
				return nil
			}
			cmd.Printf("%s: %s\n", key, value)
			return nil
		},
	}
	addOutputFlags(cmd, &plain, &asJSON)
	return cmd
}

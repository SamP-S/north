package cli

import (
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

func configValue(cfg board.Config, key string) (string, error) {
	switch key {
	case "version":
		return strconv.Itoa(cfg.Version), nil
	case "auto_commit":
		return strconv.FormatBool(cfg.AutoCommit), nil
	case "deps_enforcement":
		return string(cfg.DepsEnforcement), nil
	case "max_wip":
		return strconv.Itoa(cfg.MaxWIP), nil
	case "last_id":
		return strconv.Itoa(cfg.LastID), nil
	default:
		return "", nerrors.Invalid(fmt.Sprintf("unknown config key %q (known: %s)", key, joinKeys()))
	}
}

func joinKeys() string { return strings.Join(configKeys, ", ") }

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "print every config key and value",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadBoardConfig()
			if err != nil {
				return err
			}
			for _, key := range configKeys {
				v, err := configValue(cfg, key)
				if err != nil {
					return err
				}
				cmd.Printf("%s: %s\n", key, v)
			}
			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "print one config value",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadBoardConfig()
			if err != nil {
				return err
			}
			v, err := configValue(cfg, args[0])
			if err != nil {
				return err
			}
			cmd.Println(v)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "set one config value",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardDir, cfg, err := loadBoardConfig()
			if err != nil {
				return err
			}
			key, value := args[0], args[1]
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
				cfg.AutoCommit = b
			case "deps_enforcement":
				level, err := board.ParseDepsEnforcement(value)
				if err != nil {
					return err
				}
				cfg.DepsEnforcement = level
			case "max_wip":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					return nerrors.Invalid(fmt.Sprintf(
						"max_wip must be a non-negative integer, 0 = unlimited (got %q)", value))
				}
				cfg.MaxWIP = n
			default:
				return nerrors.Invalid(fmt.Sprintf("unknown config key %q (known: %s)", key, joinKeys()))
			}
			if _, err := board.WriteConfig(boardDir, cfg); err != nil {
				return err
			}
			cmd.Printf("%s: %s\n", key, value)
			return nil
		},
	}
}

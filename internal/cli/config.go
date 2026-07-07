package cli

import (
	"fmt"
	"strconv"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
	"github.com/spf13/cobra"
)

// configKeys lists the config keys visible to list/get. version is read-only
// (the board's format stamp); set refuses it.
var configKeys = []string{"version", "auto_commit"}

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
	default:
		return "", nerrors.Invalid(fmt.Sprintf("unknown config key %q (known: %s)", key, joinKeys()))
	}
}

func joinKeys() string {
	out := ""
	for i, k := range configKeys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}

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
			case "auto_commit":
				b, err := strconv.ParseBool(value)
				if err != nil {
					return nerrors.Invalid(fmt.Sprintf("auto_commit must be true or false (got %q)", value))
				}
				cfg.AutoCommit = b
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

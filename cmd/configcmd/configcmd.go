package configcmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

type Config struct {
	*rootcmd.RootConfig
	Command *ff.Command
	Flags   *ff.FlagSet
}

func New(parent *rootcmd.RootConfig) *Config {
	var cfg Config
	cfg.RootConfig = parent
	cfg.Flags = ff.NewFlagSet("config").SetParent(parent.Flags)

	cfg.Command = &ff.Command{
		Name:      "config",
		Usage:     "bine config <SUBCOMMAND>",
		ShortHelp: "Show configuration values.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}

	// Add get subcommand.
	getCmd := &ff.Command{
		Name:      "get",
		Usage:     "bine config get <KEY>",
		ShortHelp: "Get a configuration value.",
		Exec:      cfg.ExecGet,
	}
	cfg.Command.Subcommands = append(cfg.Command.Subcommands, getCmd)

	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	return errors.New("config command requires a subcommand (get)")
}

func (cfg *Config) ExecGet(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("config get requires one argument")
	}

	key := args[0]

	switch key {
	case "project":
		_, err := fmt.Fprintln(cfg.Stdout, cfg.Bine.Project)
		return err
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
}

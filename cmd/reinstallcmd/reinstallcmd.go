package reinstallcmd

import (
	"context"
	"errors"

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
	cfg.Flags = ff.NewFlagSet("reinstall").SetParent(parent.Flags)

	cfg.Command = &ff.Command{
		Name:      "reinstall",
		Usage:     "bine reinstall",
		ShortHelp: "Reinstall all binaries defined in the configuration file.",
		LongHelp:  "Alias for `bine sync --force`.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return errors.New("reinstall accepts no arguments")
	}

	return cfg.Bine.Reinstall(ctx)
}

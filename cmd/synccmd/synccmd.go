package synccmd

import (
	"context"

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
	cfg.Flags = ff.NewFlagSet("run").SetParent(parent.Flags)

	cfg.Command = &ff.Command{
		Name:      "sync",
		Usage:     "bine sync",
		ShortHelp: "Install all binaries defined in the configuration file.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	return cfg.Bine.Sync(ctx)
}

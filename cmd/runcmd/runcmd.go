package runcmd

import (
	"context"
	"errors"

	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
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
		Name:      "run",
		Usage:     "bine run <NAME>",
		ShortHelp: "Download a binary and run it.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("run requires one argument")
	}

	name := args[0]

	chacheDir := bine.WithCacheDir(cfg.CacheDir)
	bn, err := bine.NewWithOptions(chacheDir)
	if err != nil {
		return err
	}

	streams := bine.IOStreams{
		Stdin:  cfg.Stdin,
		Stdout: cfg.Stdout,
		Stderr: cfg.Stderr,
	}

	return bn.Run(name, args[1:], streams)
}

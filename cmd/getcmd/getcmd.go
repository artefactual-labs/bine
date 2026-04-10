package getcmd

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
	Force   bool
}

func New(parent *rootcmd.RootConfig) *Config {
	var cfg Config
	cfg.RootConfig = parent
	cfg.Flags = ff.NewFlagSet("get").SetParent(parent.Flags)
	cfg.Flags.BoolVar(&cfg.Force, 'f', "force", "Reinstall the binary even if it is already installed.")

	cfg.Command = &ff.Command{
		Name:      "get",
		Usage:     "bine get [FLAGS] <NAME>",
		ShortHelp: "Download a binary and print its path.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("get requires one argument")
	}

	name := args[0]

	var path string
	var err error
	if cfg.Force {
		path, err = cfg.Bine.GetForce(ctx, name)
	} else {
		path, err = cfg.Bine.Get(ctx, name)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(cfg.Stdout, path)

	return err
}

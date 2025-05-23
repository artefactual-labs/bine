package pathcmd

import (
	"context"
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
	cfg.Flags = ff.NewFlagSet("path").SetParent(parent.Flags)

	cfg.Command = &ff.Command{
		Name:      "path",
		Usage:     "bine path [FLAGS]",
		ShortHelp: "Print the path of the binary store.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, _ []string) error {
	_, err := fmt.Fprintln(cfg.Stdout, cfg.Bine.BinDir)

	return err
}

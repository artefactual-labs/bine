package pathcmd

import (
	"context"
	"fmt"

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
	chacheDir := bine.WithCacheDir(cfg.CacheDir)
	bn, err := bine.NewWithOptions(chacheDir)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(cfg.Stdout, bn.BinDir)

	return err
}

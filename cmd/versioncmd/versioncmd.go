package versioncmd

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

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
	cfg.Flags = ff.NewFlagSet("version").SetParent(parent.Flags)

	cfg.Command = &ff.Command{
		Name:      "version",
		Usage:     "bine version [FLAGS]",
		ShortHelp: "Print the current version of bine.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, _ []string) error {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return errors.New("build info not available")
	}

	_, err := fmt.Fprintf(cfg.Stdout, "bine %s (built with %s)\n", info.Main.Version, info.GoVersion)

	return err
}

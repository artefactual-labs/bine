package initcmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

type Config struct {
	*rootcmd.RootConfig
	Command *ff.Command
	Flags   *ff.FlagSet
	Format  string
}

func New(parent *rootcmd.RootConfig) *Config {
	cfg := &Config{RootConfig: parent}
	cfg.Flags = ff.NewFlagSet("init").SetParent(parent.Flags)
	cfg.Flags.StringVar(&cfg.Format, 0, "format", "json", "Configuration format: json or toml.")
	cfg.Command = &ff.Command{
		Name:      "init",
		Usage:     "bine init [FLAGS] [PROJECT]",
		ShortHelp: "Create a project configuration in the current directory.",
		Flags:     rootcmd.Interspersed(cfg.Flags),
		Exec:      cfg.Exec,
	}
	parent.Command.Subcommands = append(parent.Command.Subcommands, cfg.Command)
	return cfg
}

func (cfg *Config) Exec(_ context.Context, args []string) error {
	if len(args) > 1 {
		return errors.New("init accepts at most one project name")
	}
	project := ""
	if len(args) == 1 {
		project = args[0]
		if project == "" {
			return errors.New("project name cannot be empty")
		}
	}
	path, err := bine.Init(project, cfg.Format)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cfg.Stdout, "Created %s.\n", filepath.Base(path))
	return err
}

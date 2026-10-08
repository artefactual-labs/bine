package addcmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

type Config struct {
	*rootcmd.RootConfig
	Command *ff.Command
	Flags   *ff.FlagSet
	Options bine.AddOptions
}

func New(parent *rootcmd.RootConfig) *Config {
	cfg := &Config{RootConfig: parent}
	cfg.Flags = ff.NewFlagSet("add").SetParent(parent.Flags)
	cfg.Flags.StringVar(&cfg.Options.GoPackage, 0, "go", "", "Go package to build using go install.")
	cfg.Flags.StringVar(&cfg.Options.PackslipProject, 0, "packslip", "", "Signed Packslip project identity.")
	cfg.Flags.StringVar(&cfg.Options.URL, 0, "url", "", "Release source URL, such as a GitHub repository.")
	cfg.Flags.StringVar(&cfg.Options.Version, 0, "version", "latest", "Release version to pin; latest resolves an exact version.")
	cfg.Flags.StringVar(&cfg.Options.Command, 0, "command", "", "Command declared by the Packslip publisher.")
	cfg.Flags.StringVar(&cfg.Options.Variant, 0, "variant", "", "Build variant declared by the Packslip publisher.")
	cfg.Flags.StringVar(&cfg.Options.AssetPattern, 0, "asset-pattern", "", "Release asset filename template.")
	cfg.Flags.StringVar(&cfg.Options.TagPattern, 0, "tag-pattern", "", "Release tag template; defaults to v{version}.")
	cfg.Flags.BoolVar(&cfg.Options.NoInstall, 0, "no-install", "Add the version pin without installing the tool.")
	cfg.Command = &ff.Command{
		Name:      "add",
		Usage:     "bine add <NAME> [FLAGS]",
		ShortHelp: "Install a tool and add its version pin to the configuration.",
		Flags:     rootcmd.Interspersed(cfg.Flags, "no-install"),
		Exec:      cfg.Exec,
	}
	parent.Command.Subcommands = append(parent.Command.Subcommands, cfg.Command)
	return cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("add requires one binary name")
	}
	cfg.Options.Name = args[0]
	item, err := cfg.Bine.Add(ctx, cfg.Options)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cfg.Stdout, "Added %s %s.\n", item.Name, item.Version)
	return err
}

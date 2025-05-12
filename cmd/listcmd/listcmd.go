package listcmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

type Config struct {
	*rootcmd.RootConfig
	Command       *ff.Command
	Flags         *ff.FlagSet
	InstalledOnly bool
	OutdatedOnly  bool
	JSON          bool
}

func New(parent *rootcmd.RootConfig) *Config {
	var cfg Config
	cfg.RootConfig = parent
	cfg.Flags = ff.NewFlagSet("list").SetParent(parent.Flags)
	cfg.Flags.BoolVar(&cfg.InstalledOnly, 0, "installed", "List only installed binaries.")
	cfg.Flags.BoolVar(&cfg.OutdatedOnly, 0, "outdated", "List only outdated binaries.")
	cfg.Flags.BoolVar(&cfg.JSON, 0, "json", "Output in JSON format.")

	cfg.Command = &ff.Command{
		Name:      "list",
		Usage:     "bine list [FLAGS]",
		ShortHelp: "Print the list of binaries.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, _ []string) error {
	logger, _ := logr.FromContext(ctx)
	b, err := bine.NewWithOptions(
		bine.WithCacheDir(cfg.CacheDir),
		bine.WithLogger(logger),
		bine.WithGitHubAPIToken(cfg.GitHubAPIToken),
	)
	if err != nil {
		return err
	}

	items, err := b.List(ctx, cfg.InstalledOnly, cfg.OutdatedOnly)
	if err != nil {
		return err
	}

	if cfg.JSON {
		if output, err := json.MarshalIndent(items, "", "\t"); err != nil {
			return err
		} else {
			fmt.Fprintln(cfg.Stdout, string(output))
			return nil
		}
	}

	// First, print items without errors.
	for _, item := range items {
		if item.OutdatedCheckError == "" {
			line := fmt.Sprintf("%s v%s", item.Name, item.Version)
			if item.Latest != "" {
				line += fmt.Sprintf(" » v%s", item.Latest)
			}
			fmt.Fprintln(cfg.Stdout, line)
		}
	}

	// Then, print items with errors.
	for _, item := range items {
		if item.OutdatedCheckError != "" {
			line := fmt.Sprintf("%s v%s", item.Name, item.Version)
			line += fmt.Sprintf(" (%s)", item.OutdatedCheckError)
			fmt.Fprintln(cfg.Stdout, line)
		}
	}

	return nil
}

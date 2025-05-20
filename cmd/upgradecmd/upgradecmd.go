package upgradecmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

type Config struct {
	*rootcmd.RootConfig
	Command *ff.Command
	Flags   *ff.FlagSet
	DryRun  bool
}

func New(parent *rootcmd.RootConfig) *Config {
	var cfg Config
	cfg.RootConfig = parent
	cfg.Flags = ff.NewFlagSet("run").SetParent(parent.Flags)
	cfg.Flags.BoolVar(&cfg.DryRun, 0, "dry-run", "Show what would be done without actually doing it.")
	cfg.Command = &ff.Command{
		Name:      "upgrade",
		Usage:     "bine upgrade",
		ShortHelp: "Upgrade all binaries defined in the configuration file.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, args []string) error {
	logger, _ := logr.FromContext(ctx)
	b, err := bine.NewWithOptions(
		bine.WithCacheDir(cfg.CacheDir),
		bine.WithLogger(logger),
		bine.WithGitHubAPIToken(cfg.GitHubAPIToken),
	)
	if err != nil {
		return err
	}

	var upgradeFn func(ctx context.Context) ([]*bine.ListItem, error)
	if cfg.DryRun {
		upgradeFn = func(ctx context.Context) ([]*bine.ListItem, error) {
			return b.List(ctx, false, true)
		}
	} else {
		upgradeFn = func(ctx context.Context) ([]*bine.ListItem, error) {
			return b.Upgrade(ctx)
		}
	}

	updates, err := upgradeFn(ctx)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		fmt.Fprintln(cfg.Stdout, "No updates available.")
		return nil
	}

	// Halt if any binary has an outdated check error.
	for _, item := range updates {
		if item.OutdatedCheckError != "" {
			return errors.New(item.OutdatedCheckError)
		}
	}

	for _, item := range updates {
		line := fmt.Sprintf("%s %s » %s", item.Name, item.Version, item.Latest)
		fmt.Fprintln(cfg.Stdout, line)
	}

	if cfg.DryRun {
		fmt.Fprintln(cfg.Stdout, "Remove the --dry-run flag to install the updates.")
	} else {
		fmt.Fprintln(cfg.Stdout, "Upgrade process completed.")
		fmt.Fprintln(cfg.Stdout, "Review the configuration file for any errors.")
	}

	return nil
}

package rootcmd

import (
	"io"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
)

type RootConfig struct {
	Logger         logr.Logger
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	Verbosity      int
	CacheDir       string
	GitHubAPIToken string
	Flags          *ff.FlagSet
	Command        *ff.Command
	Bine           *bine.Bine
}

func New(stdin io.Reader, stdout, stderr io.Writer) *RootConfig {
	var cfg RootConfig
	cfg.Stdin = stdin
	cfg.Stdout = stdout
	cfg.Stderr = stderr
	cfg.Flags = ff.NewFlagSet("bine")
	cfg.Flags.IntVar(&cfg.Verbosity, 'v', "verbosity", -1, "Log verbosity level. The higher the number, the more verbose the output.")
	cfg.Flags.StringVar(&cfg.CacheDir, 0, "cache-dir", "", "Path to the cache directory.")
	cfg.Flags.StringVar(&cfg.GitHubAPIToken, 0, "github-api-token", "", "GitHub API token for authentication.")
	cfg.Command = &ff.Command{
		Name:      "bine",
		ShortHelp: "Simple binary manager for developers.",
		Usage:     "bine [FLAGS] <SUBCOMMAND> ...",
		Flags:     cfg.Flags,
		LongHelp: `bine helps manage external binary tools needed for development projects.

It downloads specified binaries from their sources into a local cache directory,
ensuring you have the right versions without cluttering your system.`,
	}
	return &cfg
}

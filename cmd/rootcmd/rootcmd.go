package rootcmd

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"

	"github.com/artefactual-labs/bine/bine"
)

type RootConfig struct {
	Logger         logr.Logger
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	Verbosity      int
	verboseCount   int
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
	cfg.Flags.Value('v', "verbose", (*verbosityCountValue)(&cfg.verboseCount), "Increase log verbosity. Repeat up to -vvv for the highest shorthand level.")
	cfg.Flags.IntVar(&cfg.Verbosity, 0, "verbosity", 0, "Set the log verbosity level explicitly.")
	cfg.Flags.StringVar(&cfg.CacheDir, 0, "cache-dir", "", "Path to the cache directory.")
	cfg.Flags.StringVar(&cfg.GitHubAPIToken, 0, "github-api-token", "", "GitHub API token for authentication.")
	cfg.Command = &ff.Command{
		Name:      "bine",
		ShortHelp: "Simple binary manager for developers.",
		Usage:     "bine [FLAGS] <SUBCOMMAND> ...",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
		LongHelp: `bine helps manage external binary tools needed for development projects.

It downloads specified binaries from their sources into a local cache directory,
ensuring you have the right versions without cluttering your system.`,
	}
	return &cfg
}

func (cfg *RootConfig) ResolveVerbosity() {
	if flag, ok := cfg.Flags.GetFlag("verbosity"); ok && flag.IsSet() {
		return
	}
	cfg.Verbosity = cfg.verboseCount
}

func (cfg *RootConfig) Exec(_ context.Context, args []string) error {
	if len(args) > 0 {
		fmt.Fprintf(cfg.Stdout, "%s\n", ffhelp.Command(cfg.Command))
		return fmt.Errorf("unknown subcommand %q", args[0])
	}

	fmt.Fprintf(cfg.Stdout, "%s\n", ffhelp.Command(cfg.Command))

	return ff.ErrHelp
}

// verbosityCountValue implements flag.Value for the repeatable -v/--verbose
// shorthand. ff treats values with IsBoolFlag as boolean flags, which lets
// clustered forms like -vv and repeated long forms like --verbose --verbose
// increment the counter once per occurrence.
type verbosityCountValue int

func (v *verbosityCountValue) String() string {
	if v == nil {
		return "0"
	}
	return strconv.Itoa(int(*v))
}

func (v *verbosityCountValue) Set(value string) error {
	switch value {
	case "", "true":
		*v = *v + 1
		return nil
	case "false":
		*v = 0
		return nil
	default:
		return fmt.Errorf("invalid boolean value %q", value)
	}
}

// IsBoolFlag tells ff to parse -v and --verbose without requiring an explicit
// value, while still routing each occurrence through Set.
func (v *verbosityCountValue) IsBoolFlag() bool {
	return true
}

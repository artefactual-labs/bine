package rootcmd

import (
	"io"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffval"
)

type RootConfig struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Verbose  bool
	CacheDir string
	Flags    *ff.FlagSet
	Command  *ff.Command
}

func New(stdin io.Reader, stdout, stderr io.Writer) *RootConfig {
	var cfg RootConfig
	cfg.Stdin = stdin
	cfg.Stdout = stdout
	cfg.Stderr = stderr
	cfg.Flags = ff.NewFlagSet("objectctl")
	cfg.Flags.StringVar(&cfg.CacheDir, 0, "cache-dir", "", "Path to the cache directory.")
	cfg.Flags.AddFlag(ff.FlagConfig{
		ShortName: 'v',
		LongName:  "verbose",
		Value:     ffval.NewValue(&cfg.Verbose),
		Usage:     "log verbose output",
		NoDefault: true,
	})
	cfg.Command = &ff.Command{
		Name:      "bine",
		ShortHelp: "Simple binary manager for developers.",
		Usage:     "bine [FLAGS] <SUBCOMMAND> ...",
		Flags:     cfg.Flags,
		LongHelp:  "asdf",
	}
	return &cfg
}

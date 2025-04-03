package rootcmd

import (
	"io"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffval"
)

type RootConfig struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
	Flags   *ff.FlagSet
	Command *ff.Command
}

func New(stdout, stderr io.Writer) *RootConfig {
	var cfg RootConfig
	cfg.Stdout = stdout
	cfg.Stderr = stderr
	cfg.Flags = ff.NewFlagSet("objectctl")
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

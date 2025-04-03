package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"

	"github.com/artefactual-labs/bine/cmd/getcmd"
	"github.com/artefactual-labs/bine/cmd/pathcmd"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
	"github.com/artefactual-labs/bine/cmd/runcmd"
	"github.com/artefactual-labs/bine/cmd/versioncmd"
)

func main() {
	var (
		ctx    = context.Background()
		args   = os.Args[1:]
		stdout = os.Stdout
		stderr = os.Stderr
		err    = exec(ctx, args, stdout, stderr)
	)
	switch {
	case err == nil, errors.Is(err, ff.ErrHelp), errors.Is(err, ff.ErrNoExec):
		// Nothing to do.
	case err != nil:
		fmt.Fprintf(stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func exec(ctx context.Context, args []string, stdout, stderr io.Writer) (err error) {
	var (
		root = rootcmd.New(stdout, stderr)
		_    = getcmd.New(root)
		_    = pathcmd.New(root)
		_    = runcmd.New(root)
		_    = versioncmd.New(root)
	)

	defer func() {
		if err != nil {
			fmt.Fprintf(stderr, "\n%s\n", ffhelp.Command(root.Command))
		}
	}()

	if err := root.Command.Parse(args); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := root.Command.Run(ctx); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

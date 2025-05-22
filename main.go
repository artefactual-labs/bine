package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"go.artefactual.dev/tools/log"

	"github.com/artefactual-labs/bine/cmd/envcmd"
	"github.com/artefactual-labs/bine/cmd/getcmd"
	"github.com/artefactual-labs/bine/cmd/listcmd"
	"github.com/artefactual-labs/bine/cmd/pathcmd"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
	"github.com/artefactual-labs/bine/cmd/runcmd"
	"github.com/artefactual-labs/bine/cmd/synccmd"
	"github.com/artefactual-labs/bine/cmd/upgradecmd"
	"github.com/artefactual-labs/bine/cmd/versioncmd"
)

func main() {
	var (
		ctx    = context.Background()
		args   = os.Args[1:]
		stdin  = os.Stdin
		stdout = os.Stdout
		stderr = os.Stderr
		err    = exec(ctx, args, stdin, stdout, stderr)
	)
	switch {
	case err == nil, errors.Is(err, ff.ErrHelp), errors.Is(err, ff.ErrNoExec):
		// Nothing to do.
	default:
		// Special handling for exec errors when running commands.
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		fmt.Fprintf(stderr, "Command failed: %v.\n", err)
		os.Exit(1)
	}
}

func exec(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	var (
		root = rootcmd.New(stdin, stdout, stderr)
		_    = envcmd.New(root)
		_    = getcmd.New(root)
		_    = listcmd.New(root)
		_    = pathcmd.New(root)
		_    = runcmd.New(root)
		_    = synccmd.New(root)
		_    = upgradecmd.New(root)
		_    = versioncmd.New(root)
	)

	opts := []ff.Option{
		ff.WithEnvVarPrefix("BINE"),
	}
	if err := root.Command.Parse(args, opts...); err != nil {
		fmt.Fprintf(stderr, "\n%s\n", ffhelp.Command(root.Command))
		return err
	}

	var logger logr.Logger
	if root.Verbosity > 0 {
		logger = log.New(
			os.Stderr,
			log.WithName("bine"),
			log.WithDebug(true),
			log.WithLevel(root.Verbosity),
		)
		defer log.Sync(logger)
	}

	logger.V(1).Info("Starting bine.")

	logger = logger.WithName(root.Command.GetSelected().Name)
	logger.V(1).Info("Running command.", "args", args)

	ctx = logr.NewContext(ctx, logger) // Pass the logger via context.
	if err := root.Command.Run(ctx); err != nil {
		return err
	}

	return nil
}

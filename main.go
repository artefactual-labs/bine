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

	"github.com/artefactual-labs/bine/bine"
	"github.com/artefactual-labs/bine/cmd/configcmd"
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
	case err == nil, errors.Is(err, ff.ErrHelp):
		// Nothing to do.
	default:
		// Special handling for exec errors when running commands.
		if code := exitError(err); code > -1 {
			os.Exit(code)
		}

		fmt.Fprintf(stderr, "Command failed: %v.\n", err)
		os.Exit(1)
	}
}

func exec(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	var (
		root = rootcmd.New(stdin, stdout, stderr)
		_    = configcmd.New(root)
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
	cmd := root.Command.GetSelected().Name

	// Skip building for help/version.
	if cmd != "version" && cmd != root.Command.Name {
		if b, err := build(ctx, logger, root); err != nil {
			return err
		} else {
			root.Bine = b
		}
	}

	logger = logger.WithName(cmd)
	logger.V(1).Info("Running command.", "args", args)
	if err := root.Command.Run(ctx); err != nil {
		return err
	}

	return nil
}

func build(ctx context.Context, logger logr.Logger, root *rootcmd.RootConfig) (*bine.Bine, error) {
	return bine.NewWithOptions( //nolint:contextcheck // Use bine.WithContext.
		bine.WithContext(ctx),
		bine.WithCacheDir(root.CacheDir),
		bine.WithLogger(logger),
		bine.WithGitHubAPIToken(root.GitHubAPIToken),
	)
}

func exitError(err error) int {
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

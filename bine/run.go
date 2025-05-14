package bine

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

type IOStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// run executes a command with the given arguments and streams.
//
// This is the core of `bine run`. Inspired by `go tool`.
func run(ctx context.Context, path string, args []string, streams IOStreams) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = streams.Stdin
	cmd.Stdout = streams.Stdout
	cmd.Stderr = streams.Stderr

	err := cmd.Start()
	if err == nil {
		c := make(chan os.Signal, 100)
		signal.Notify(c)
		go func() {
			for sig := range c {
				_ = cmd.Process.Signal(sig)
			}
		}()
		err = cmd.Wait()
		signal.Stop(c)
		close(c)
	}

	return err
}

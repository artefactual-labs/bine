package bine

import (
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

func run(path string, args []string, streams IOStreams) error {
	cmd := exec.Command(path, args...)
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

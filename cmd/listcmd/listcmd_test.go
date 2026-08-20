package listcmd

import (
	"io"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

func TestCheckIntervalFlagAfterSubcommand(t *testing.T) {
	root := rootcmd.New(nil, io.Discard, io.Discard)
	cfg := New(root)

	err := root.Command.Parse([]string{"list", "--outdated", "--check-interval=1s"})
	assert.NilError(t, err)
	assert.Equal(t, cfg.CheckInterval, time.Second)
}

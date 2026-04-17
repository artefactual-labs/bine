package rootcmd

import (
	"io"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestResolveVerbosity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		verbosity int
		wantErr   string
	}{
		{name: "default", verbosity: 0},
		{name: "single verbose", args: []string{"-v"}, verbosity: 1},
		{name: "double verbose", args: []string{"-vv"}, verbosity: 2},
		{name: "triple verbose", args: []string{"-vvv"}, verbosity: 3},
		{name: "long verbose", args: []string{"--verbose", "--verbose"}, verbosity: 2},
		{name: "explicit verbosity", args: []string{"--verbosity=2"}, verbosity: 2},
		{name: "explicit verbosity wins", args: []string{"-vv", "--verbosity=1"}, verbosity: 1},
		{name: "explicit zero wins", args: []string{"-vv", "--verbosity=0"}, verbosity: 0},
		{name: "verbose rejects explicit value", args: []string{"--verbose=2"}, wantErr: "invalid boolean value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := New(strings.NewReader(""), io.Discard, io.Discard)
			err := cfg.Command.Parse(tt.args)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)

			cfg.ResolveVerbosity()

			assert.Equal(t, cfg.Verbosity, tt.verbosity)
		})
	}
}

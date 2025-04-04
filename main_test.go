package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestPath(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"path"}, stdin, stdout, stderr)
	assert.NilError(t, err)
	assert.Assert(t, true == strings.HasSuffix(stdout.String(), filepath.Join(runtime.GOOS, runtime.GOARCH, "bin")+"\n"))
}

func TestRun(t *testing.T) {
	t.Parallel()

	var (
		ctx   = context.Background()
		stdin = strings.NewReader(`{
  "Path": "github.com/mholt/archives",
  "Version": "v0.1.0",
  "Time": "2024-12-26T19:40:06Z",
  "Update": {
    "Path": "github.com/mholt/archives",
    "Version": "v0.1.1",
    "Time": "2025-04-02T02:26:02Z"
  },
  "Dir": "/home/ethan/go/pkg/mod/github.com/mholt/archives@v0.1.0",
  "GoMod": "/home/ethan/go/pkg/mod/cache/download/github.com/mholt/archives/@v/v0.1.0.mod",
  "GoVersion": "1.22.2",
  "Sum": "h1:FacgJyrjiuyomTuNA92X5GyRBRZjE43Y/lrzKIlF35Q=",
  "GoModSum": "h1:j/Ire/jm42GN7h90F5kzj6hf6ZFzEH66de+hmjEKu+I="
}`)
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"run", "go-mod-outdated", "-ci"}, stdin, stdout, stderr)
	assert.Error(t, err, "run: exit status 1")
	assert.Assert(t, errors.As(err, new(*osexec.ExitError)))
}

func TestVersion(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"version"}, stdin, stdout, stderr)
	assert.NilError(t, err)

	info, _ := debug.ReadBuildInfo()
	assert.Equal(t, stdout.String(), fmt.Sprintf("bine %s (built with %s)\n", info.Main.Version, info.GoVersion))
}

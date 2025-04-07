package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestPath(t *testing.T) {
	var (
		ctx    = context.Background()
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"path"}, stdin, stdout, stderr)
	assert.NilError(t, err)
	assert.Equal(t, trimmed(t, stdout), binPath(t, ""))
}

func TestGet(t *testing.T) {
	var (
		ctx    = context.Background()
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"get", "tparse"}, stdin, stdout, stderr)
	assert.NilError(t, err)
	assert.Equal(t, trimmed(t, stdout), binPath(t, "tparse"))
}

func TestRun(t *testing.T) {
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
	assert.Error(t, err, "run: run: exit status 1")
	assert.Assert(t, errors.As(err, new(*osexec.ExitError)))
}

func TestVersion(t *testing.T) {
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

func trimmed(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()

	return strings.TrimSuffix(buf.String(), "\n")
}

func binPath(t *testing.T, name string) string {
	t.Helper()

	cacheDir, err := os.UserCacheDir()
	assert.NilError(t, err)

	return filepath.Join(cacheDir, "bine", "bine", runtime.GOOS, runtime.GOARCH, "bin", name)
}

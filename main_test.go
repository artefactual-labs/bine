package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// NOTE: these tests are not reusing your cache directory. It's using the root
// flag `--cache-dir` to specify a temporary directory for each test. Further
// work should inject custom config files as opposed to relying on the project's
// default configuration.

func TestPath(t *testing.T) {
	var (
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	cacheDir, err := runExec(t, []string{"path"}, stdin, stdout, stderr)
	assert.NilError(t, err)
	assert.Equal(t, trimmed(t, stdout), binPath(t, cacheDir, ""))
}

func TestGet(t *testing.T) {
	var (
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	cacheDir, err := runExec(t, []string{"get", "tparse"}, stdin, stdout, stderr)
	assert.NilError(t, err)
	assert.Equal(t, trimmed(t, stdout), binPath(t, cacheDir, "tparse"))
}

func TestRun(t *testing.T) {
	var (
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

	_, err := runExec(t, []string{"run", "go-mod-outdated", "-ci"}, stdin, stdout, stderr)
	assert.Error(t, err, "run: run: exit status 1")
	assert.Assert(t, errors.As(err, new(*osexec.ExitError)))
}

func TestSync(t *testing.T) {
	var (
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	_, err := runExec(t, []string{"sync"}, stdin, stdout, stderr)
	assert.NilError(t, err)
	assert.Equal(t, stdout.String(), "")
	assert.Equal(t, stderr.String(), "")
}

func TestVersion(t *testing.T) {
	var (
		stdin  = strings.NewReader("")
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	_, err := runExec(t, []string{"version"}, stdin, stdout, stderr)
	assert.NilError(t, err)

	info, _ := debug.ReadBuildInfo()
	assert.Equal(t, stdout.String(), fmt.Sprintf("bine %s (built with %s)\n", info.Main.Version, info.GoVersion))
}

func runExec(t *testing.T, args []string, stdin io.Reader, stdout, stderr *bytes.Buffer) (string, error) {
	t.Helper()

	var (
		ctx      = t.Context()
		tempDir  = t.TempDir()
		fullArgs = append([]string{"--cache-dir", tempDir}, args...)
	)

	return tempDir, exec(ctx, fullArgs, stdin, stdout, stderr)
}

func trimmed(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()

	return strings.TrimSuffix(buf.String(), "\n")
}

func binPath(t *testing.T, cacheDir, name string) string {
	t.Helper()

	return filepath.Join(cacheDir, "bine", runtime.GOOS, runtime.GOARCH, "bin", name)
}

package main

import (
	"bytes"
	"context"
	"fmt"
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
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"path"}, stdout, stderr)
	assert.NilError(t, err)
	assert.Assert(t, true == strings.HasSuffix(stdout.String(), filepath.Join(runtime.GOOS, runtime.GOARCH, "bin")+"\n"))
}

func TestVersion(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	)

	err := exec(ctx, []string{"version"}, stdout, stderr)
	assert.NilError(t, err)

	info, _ := debug.ReadBuildInfo()
	assert.Equal(t, stdout.String(), fmt.Sprintf("bine %s (built with %s)\n", info.Main.Version, info.GoVersion))
}

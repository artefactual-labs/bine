package bine_test

import (
	"testing"

	"github.com/go-logr/logr"
	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/bine"
)

func TestNew(t *testing.T) {
	t.Parallel()

	bine, err := bine.New()
	assert.NilError(t, err)
	assert.Assert(t, bine.BinDir != "")
	assert.Assert(t, bine.CacheDir != "")
	assert.Assert(t, bine.VersionsDir != "")

	_, err = bine.List(t.Context(), true, false)
	assert.NilError(t, err)
}

func TestNewWithOptions(t *testing.T) {
	t.Parallel()

	bine, err := bine.NewWithOptions(
		bine.WithCacheDir(t.TempDir()),
		bine.WithLogger(logr.Discard()),
		bine.WithGitHubAPIToken("token"),
	)
	assert.NilError(t, err)
	assert.Assert(t, bine.BinDir != "")
	assert.Assert(t, bine.CacheDir != "")
	assert.Assert(t, bine.VersionsDir != "")

	listed, err := bine.List(t.Context(), true, false)
	assert.NilError(t, err)
	assert.Equal(t, len(listed), 0, "expected no bins to be listed")
}

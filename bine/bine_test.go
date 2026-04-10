package bine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"gotest.tools/v3/assert"
)

func TestNew(t *testing.T) {
	t.Parallel()

	bine, err := New()
	assert.NilError(t, err)
	assert.Assert(t, bine.BinDir != "")
	assert.Assert(t, bine.CacheDir != "")
	assert.Assert(t, bine.VersionsDir != "")

	_, err = bine.List(t.Context(), true, false)
	assert.NilError(t, err)
}

func TestNewWithOptions(t *testing.T) {
	t.Parallel()

	bine, err := NewWithOptions(
		WithCacheDir(t.TempDir()),
		WithLogger(logr.Discard()),
		WithGitHubAPIToken("token"),
	)
	assert.NilError(t, err)
	assert.Assert(t, bine.BinDir != "")
	assert.Assert(t, bine.CacheDir != "")
	assert.Assert(t, bine.VersionsDir != "")

	listed, err := bine.List(t.Context(), true, false)
	assert.NilError(t, err)
	assert.Equal(t, len(listed), 0, "expected no bins to be listed")
}

func newForceTestBine(t *testing.T) (*Bine, *bin) {
	t.Helper()

	cacheDir := t.TempDir()
	tool := &bin{
		Name:      "tool",
		GoPackage: "github.com/foo/bar/cmd/tool",
		Version:   "1.0.0",
	}

	return &Bine{
		BinDir:      filepath.Join(cacheDir, "bin"),
		VersionsDir: filepath.Join(cacheDir, "versions"),
		config: &config{
			Bins: []*bin{tool},
		},
	}, tool
}

func TestGetForceReinstallsExistingBinary(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithCounter")

	counterPath := filepath.Join(t.TempDir(), "counter")
	t.Setenv("BINE_HELPER_COUNTER", counterPath)

	b, tool := newForceTestBine(t)

	path, err := b.Get(t.Context(), tool.Name)
	assert.NilError(t, err)

	blob, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-1")

	path, err = b.Get(t.Context(), tool.Name)
	assert.NilError(t, err)

	blob, err = os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-1")

	path, err = b.GetForce(t.Context(), tool.Name)
	assert.NilError(t, err)

	blob, err = os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-2")
}

func TestSyncForceReinstallsExistingBinaries(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithCounter")

	counterPath := filepath.Join(t.TempDir(), "counter")
	t.Setenv("BINE_HELPER_COUNTER", counterPath)

	b, tool := newForceTestBine(t)
	path := filepath.Join(b.BinDir, tool.Name)

	err := b.Sync(t.Context())
	assert.NilError(t, err)

	blob, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-1")

	err = b.Sync(t.Context())
	assert.NilError(t, err)

	blob, err = os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-1")

	err = b.SyncForce(t.Context())
	assert.NilError(t, err)

	blob, err = os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-2")

	err = b.Reinstall(t.Context())
	assert.NilError(t, err)

	blob, err = os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary-3")
}

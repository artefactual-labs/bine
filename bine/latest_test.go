package bine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

type staticProvider struct {
	latest string
	err    error
}

func (p staticProvider) downloadURL(*bin) (string, error) {
	return "", nil
}

func (p staticProvider) latestVersion(context.Context, *bin) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.latest, nil
}

func newLatestTrackingTestBine(t *testing.T, latest string) (*Bine, *bin) {
	t.Helper()

	cacheDir := t.TempDir()
	b := &Bine{
		BinDir:      filepath.Join(cacheDir, "bin"),
		VersionsDir: filepath.Join(cacheDir, "versions"),
	}

	assert.NilError(t, os.MkdirAll(b.BinDir, 0o750))

	configPath := filepath.Join(cacheDir, ".bine.json")
	assert.NilError(t, os.WriteFile(configPath, []byte("{}"), 0o640))

	tool := &bin{
		Name:      "tool",
		GoPackage: "github.com/foo/bar/cmd/tool",
		Version:   "latest",
		provider:  staticProvider{latest: latest},
	}

	b.config = &config{
		path: configPath,
		Bins: []*bin{tool},
	}

	return b, tool
}

func writeLatestTrackingBinary(t *testing.T, b *Bine, bin *bin) string {
	t.Helper()

	binPath := filepath.Join(b.BinDir, bin.Name)
	assert.NilError(t, os.WriteFile(binPath, []byte("binary"), 0o755))

	return binPath
}

func TestInstalledRepairsLatestMarker(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessGoVersionM")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	writeLatestTrackingBinary(t, b, bin)
	assert.NilError(t, b.markVersion(bin, ""))

	ok, err := b.installed(t.Context(), bin)
	assert.NilError(t, err)
	assert.Assert(t, ok)

	marker, err := b.readVersionMarker(bin)
	assert.NilError(t, err)
	assert.Equal(t, marker.ResolvedVersion, "1.2.3")
}

func TestInstalledReturnsFalseWhenLatestVersionCannotBeResolved(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithError")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	writeLatestTrackingBinary(t, b, bin)
	assert.NilError(t, b.markVersion(bin, ""))

	ok, err := b.installed(t.Context(), bin)
	assert.NilError(t, err)
	assert.Assert(t, !ok)
}

func TestListOutdatedRepairsLatestResolvedVersion(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessGoVersionM")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	writeLatestTrackingBinary(t, b, bin)
	assert.NilError(t, b.markVersion(bin, ""))

	items, err := b.List(t.Context(), false, true)
	assert.NilError(t, err)
	assert.Equal(t, len(items), 1)
	assert.Equal(t, items[0].Name, "tool")
	assert.Equal(t, items[0].Version, "v1.2.3")
	assert.Equal(t, items[0].Latest, "v2.0.0")
	assert.Equal(t, items[0].OutdatedCheckError, "")

	marker, err := b.readVersionMarker(bin)
	assert.NilError(t, err)
	assert.Equal(t, marker.ResolvedVersion, "1.2.3")
}

func TestListOutdatedSkipsUninstalledLatestBins(t *testing.T) {
	b, _ := newLatestTrackingTestBine(t, "2.0.0")

	items, err := b.List(t.Context(), false, true)
	assert.NilError(t, err)
	assert.Equal(t, len(items), 0)
}

func TestUpgradeFailsSafelyWhenLatestMarkerCannotBeRemoved(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessGoVersionM")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	binPath := writeLatestTrackingBinary(t, b, bin)

	versionMarker := filepath.Join(b.VersionsDir, bin.Name, bin.markerVersion())
	assert.NilError(t, os.MkdirAll(versionMarker, 0o750))
	assert.NilError(t, os.WriteFile(filepath.Join(versionMarker, "keep"), []byte("x"), 0o640))

	updates, err := b.Upgrade(t.Context())
	assert.ErrorContains(t, err, "remove version marker")
	assert.Equal(t, len(updates), 1)

	_, statErr := os.Stat(binPath)
	assert.NilError(t, statErr)
}

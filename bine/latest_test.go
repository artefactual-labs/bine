package bine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

type staticSource struct {
	goSource
	latest string
	err    error
}

func (p staticSource) install(ctx context.Context, request installRequest, target string) (installResult, error) {
	return p.goSource.install(ctx, request, target)
}

func (p staticSource) validateMarker(ctx context.Context, b *bin, marker *versionMarkerDocument) (bool, error) {
	return p.goSource.validateMarker(ctx, b, marker)
}

func (p staticSource) latestVersion(context.Context, *bin) (string, error) {
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
		source:    staticSource{latest: latest},
	}

	b.config = &config{
		path:   configPath,
		format: configFormatJSON,
		Bins:   []*bin{tool},
	}

	return b, tool
}

func writeLatestTrackingBinary(t *testing.T, b *Bine, bin *bin) string {
	t.Helper()

	binPath := filepath.Join(b.BinDir, bin.Name)
	assert.NilError(t, os.WriteFile(binPath, []byte("binary"), 0o755))

	return binPath
}

func TestGetRecoversLatestAfterMarkerFailure(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithSuccess")
	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	markerPath := filepath.Join(b.VersionsDir, bin.Name, "latest")
	installationBlock(t, markerPath)
	_, err := b.Get(t.Context(), bin.Name)
	assert.ErrorContains(t, err, markerPath)
	binPath := filepath.Join(b.BinDir, bin.Name)
	assert.Equal(t, string(installationRead(t, binPath)), "binary")
	// Embedded version recovery succeeds even while marker repair is blocked.
	injectFakeExec(t, "TestHelperProcessGoVersionM")
	path, err := b.Get(t.Context(), bin.Name)
	assert.NilError(t, err)
	assert.Equal(t, path, binPath)
	assert.NilError(t, os.RemoveAll(markerPath))
	_, err = b.Get(t.Context(), bin.Name)
	assert.NilError(t, err)
	marker, err := b.readVersionMarker(bin)
	assert.NilError(t, err)
	assert.Equal(t, marker.ResolvedVersion, "1.2.3")
	assert.Equal(t, bin.Version, "latest")
	assert.Equal(t, string(installationRead(t, b.config.path)), "{}")
}

func TestInstalledReturnsFalseWhenLatestVersionCannotBeResolved(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithError")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	writeLatestTrackingBinary(t, b, bin)
	assert.NilError(t, b.markVersion(bin, installResult{}))

	ok, err := b.installed(t.Context(), bin)
	assert.NilError(t, err)
	assert.Assert(t, !ok)
}

func TestGetForceLatestSucceedsWhenResolvedVersionCannotBeDetermined(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessInstallButFailGoVersionM")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	binPath := writeLatestTrackingBinary(t, b, bin)
	assert.NilError(t, b.markVersion(bin, installResult{}))

	path, err := b.GetForce(t.Context(), bin.Name)
	assert.NilError(t, err)
	assert.Equal(t, path, binPath)

	blob, err := os.ReadFile(binPath)
	assert.NilError(t, err)
	assert.Equal(t, string(blob), "binary")
}

func TestListOutdatedRepairsLatestResolvedVersion(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessGoVersionM")

	b, bin := newLatestTrackingTestBine(t, "2.0.0")
	writeLatestTrackingBinary(t, b, bin)
	assert.NilError(t, b.markVersion(bin, installResult{}))

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

func TestListOneScopesOutdatedChecksToTargetBin(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".bine.json")
	assert.NilError(t, os.WriteFile(configPath, []byte("{}"), 0o640))

	b := &Bine{
		config: &config{
			path:   configPath,
			format: configFormatJSON,
			Bins: []*bin{
				{
					Name:      "broken",
					GoPackage: "github.com/foo/bar/cmd/broken",
					Version:   "1.0.0",
					source:    staticSource{err: errors.New("boom")},
				},
				{
					Name:      "tool",
					GoPackage: "github.com/foo/bar/cmd/tool",
					Version:   "1.0.0",
					source:    staticSource{latest: "2.0.0"},
				},
			},
		},
	}

	items, err := b.ListOne(t.Context(), "tool", false, true)
	assert.NilError(t, err)
	assert.Equal(t, len(items), 1)
	assert.Equal(t, items[0].Name, "tool")
	assert.Equal(t, items[0].Latest, "v2.0.0")
}

func TestUpgradeOneOnlyUpdatesRequestedBin(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithSuccess")

	cacheDir := t.TempDir()
	configPath := filepath.Join(cacheDir, ".bine.json")
	assert.NilError(t, os.WriteFile(configPath, []byte(`{
  "project": "test",
  "bins": [
    {"name": "tool", "go_package": "github.com/foo/bar/cmd/tool", "version": "1.0.0"},
    {"name": "other", "go_package": "github.com/foo/bar/cmd/other", "version": "1.0.0"}
  ]
}`), 0o640))

	b := &Bine{
		BinDir:      filepath.Join(cacheDir, "bin"),
		VersionsDir: filepath.Join(cacheDir, "versions"),
		config: &config{
			path:   configPath,
			format: configFormatJSON,
			Bins: []*bin{
				{
					Name:      "tool",
					GoPackage: "github.com/foo/bar/cmd/tool",
					Version:   "1.0.0",
					source:    staticSource{latest: "2.0.0"},
				},
				{
					Name:      "other",
					GoPackage: "github.com/foo/bar/cmd/other",
					Version:   "1.0.0",
					source:    staticSource{latest: "3.0.0"},
				},
			},
		},
	}

	updates, err := b.UpgradeOne(t.Context(), "tool")
	assert.NilError(t, err)
	assert.Equal(t, len(updates), 1)
	assert.Equal(t, updates[0].Name, "tool")
	assert.Equal(t, updates[0].Latest, "v2.0.0")

	configBlob, err := os.ReadFile(configPath)
	assert.NilError(t, err)
	assert.Equal(t, string(configBlob), `{
  "project": "test",
  "bins": [
    {"name": "tool", "go_package": "github.com/foo/bar/cmd/tool", "version": "2.0.0"},
    {"name": "other", "go_package": "github.com/foo/bar/cmd/other", "version": "1.0.0"}
  ]
}`)

	_, err = os.Stat(filepath.Join(b.BinDir, "tool"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(b.BinDir, "other"))
	assert.Assert(t, os.IsNotExist(err))
}

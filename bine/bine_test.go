package bine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/internal/packslip"
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

func TestWithCheckIntervalRejectsNegativeDuration(t *testing.T) {
	var opts options

	err := WithCheckInterval(-time.Second)(&opts)
	assert.Error(t, err, "check interval cannot be negative")
}

func newForceTestBine(t *testing.T) (*Bine, *bin) {
	t.Helper()

	cacheDir := t.TempDir()
	tool := &bin{
		Name:      "tool",
		GoPackage: "github.com/foo/bar/cmd/tool",
		Version:   "1.0.0",
		source:    &goSource{},
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

// Only upstream responses and signature verification are replaced. Installation,
// artifact checks, trust acceptance, receipts, and configuration writes are real.
type installationFixture struct {
	b                          *Bine
	dir, stateDir, downloadDir string
	configPath                 string
	downloads                  map[string]int
	fail                       map[string]bool
	onDownload                 func()
}

type installationTransport func(*http.Request) (*http.Response, error)

func (f installationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newInstallationFixture(t *testing.T, sources ...string) *installationFixture {
	t.Helper()
	return newInstallationFixtureForHost(t, packslip.Host{}, sources...)
}

func newInstallationFixtureForHost(t *testing.T, host packslip.Host, sources ...string) *installationFixture {
	t.Helper()
	f := &installationFixture{dir: t.TempDir(), downloads: map[string]int{}, fail: map[string]bool{}}
	t.Chdir(f.dir)
	f.stateDir, f.downloadDir = filepath.Join(f.dir, "trust"), filepath.Join(f.dir, "downloads")
	assert.NilError(t, os.Mkdir(f.downloadDir, 0o700))
	t.Setenv("TMPDIR", f.downloadDir)
	f.configPath = filepath.Join(f.dir, ".bine.toml")
	responses, artifacts := map[string][]byte{}, map[string]string{}
	type verifiedBundle struct {
		payload  []byte
		identity packslip.Identity
	}
	bundles := map[string]verifiedBundle{}
	var config strings.Builder
	config.WriteString("project = 'test'\n")
	for i, source := range sources {
		name := fmt.Sprintf("tool%d", i+1)
		project := "github.com/example/" + name
		repo := "https://" + project
		fmt.Fprintf(&config, "[[bins]]\nname = '%s'\nversion = '1.0.0'\n", name)
		if source == "recipe" {
			fmt.Fprintf(&config, "url = '%s'\nasset_pattern = '{name}'\n", repo)
		} else {
			fmt.Fprintf(&config, "packslip = { project = '%s' }\n", project)
		}
		api := "https://api.github.com/repos/example/" + name
		responses[api] = []byte(fmt.Sprintf(`{"id":%d,"owner":{"id":456},"full_name":"example/%s","default_branch":"main"}`, i+100, name))
		var releases []map[string]any
		for _, version := range []string{"2.0.0", "1.0.0"} {
			content := []byte(name + "@" + version)
			url := repo + "/releases/download/v" + version + "/" + name
			if source == "packslip" {
				// Long URL query strings must not become filesystem names.
				url += "?token=" + strings.Repeat("x", 300)
			}
			responses[url], artifacts[url] = content, name+"@"+version
			bundleURL := repo + "/releases/download/v" + version + "/packslip.sigstore.json"
			releases = append(releases, map[string]any{"tag_name": "v" + version, "assets": []map[string]string{{"name": "packslip.sigstore.json", "browser_download_url": bundleURL}}})
			id := packslip.Identity{Repository: repo, RepositoryID: fmt.Sprint(i + 100), OwnerID: "456", Signer: repo + "/.github/workflows/release.yml@refs/tags/v" + version}
			size, sum := int64(len(content)), sha256.Sum256(content)
			payload := installationJSON(t, map[string]any{
				"_type": "https://in-toto.io/Statement/v1", "predicateType": packslip.ReleaseType,
				"subject": []packslip.Subject{{Name: name, Digest: map[string]string{"sha256": hex.EncodeToString(sum[:])}}},
				"predicate": map[string]any{
					"project": project, "version": version, "published_at": "2026-09-01T00:00:00Z",
					"identity":  map[string]string{"scheme": "sigstore-oidc", "issuer": "https://token.actions.githubusercontent.com", "key_id": id.Signer},
					"artifacts": []packslip.Artifact{{Name: name, OS: host.OS, Arch: host.Arch, Libc: host.Libc, Size: &size, URL: url, Format: "raw", Bin: []packslip.Executable{{Path: name, Name: name}}}},
				},
			})
			bundle := installationJSON(t, map[string]any{"dsseEnvelope": map[string]string{"payload": base64.StdEncoding.EncodeToString(payload)}})
			responses[bundleURL], bundles[string(bundle)] = bundle, verifiedBundle{payload, id}
		}
		responses[api+"/releases?per_page=100&page=1"] = installationJSON(t, releases)
		responses[api+"/releases"] = installationJSON(t, releases)
	}
	assert.NilError(t, os.WriteFile(f.configPath, []byte(config.String()), 0o600))
	client := &http.Client{Transport: installationTransport(func(req *http.Request) (*http.Response, error) {
		data, ok := responses[req.URL.String()]
		status := http.StatusOK
		if !ok {
			status = http.StatusNotFound
		}
		if key, artifact := artifacts[req.URL.String()]; artifact {
			f.downloads[key]++
			if f.onDownload != nil {
				f.onDownload()
			}
			if f.fail[key] {
				status = http.StatusServiceUnavailable
			}
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), Request: req}, nil
	})}
	cfg, err := loadConfig(t.Context(), sourceFactory{client: client, stateDir: f.stateDir})
	assert.NilError(t, err)
	for _, bin := range cfg.Bins {
		if source, ok := bin.source.(*packslipSource); ok {
			source.verifyBundle = func(_ context.Context, data []byte) ([]byte, packslip.Identity, error) {
				bundle, ok := bundles[string(data)]
				if !ok {
					return nil, packslip.Identity{}, fmt.Errorf("unrecognized test bundle")
				}
				return bundle.payload, bundle.identity, nil
			}
		}
	}
	f.b = &Bine{config: cfg, BinDir: filepath.Join(f.dir, "bin"), VersionsDir: filepath.Join(f.dir, "versions")}
	return f
}

func installationJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	assert.NilError(t, err)
	return data
}

func installationRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NilError(t, err)
	return data
}

func installationBlock(t *testing.T, path string) {
	t.Helper()
	assert.NilError(t, os.MkdirAll(path, 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(path, "keep"), nil, 0o600))
}

func (f *installationFixture) assertClean(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(f.b.BinDir)
	assert.NilError(t, err)
	for _, entry := range entries {
		assert.Assert(t, !strings.HasPrefix(entry.Name(), ".bine-"), "left staging file %s", entry.Name())
	}
	entries, err = os.ReadDir(f.downloadDir)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 0, "download files must be cleaned up")
}

func TestPackslipGetChecksCachedArtifactHost(t *testing.T) {
	for _, tc := range []struct{ goarch, arch string }{
		{"amd64", "x86_64"},
		{"loong64", "loongarch64"},
	} {
		t.Run(tc.goarch, func(t *testing.T) {
			previousOS, previousArch, previousExec := goos, goarch, execCommand
			goos, goarch = "linux", tc.goarch
			libc := "GNU libc 2.39"
			execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "sh", "-c", `printf '%s' "$1"`, "probe", libc)
			}
			t.Cleanup(func() { goos, goarch, execCommand = previousOS, previousArch, previousExec })
			f := newInstallationFixtureForHost(t, packslip.Host{OS: "linux", Arch: tc.arch, Libc: "gnu"}, "packslip")
			path, err := f.b.Get(t.Context(), "tool1")
			assert.NilError(t, err)
			_, err = f.b.Get(t.Context(), "tool1")
			assert.NilError(t, err)
			assert.Equal(t, f.downloads["tool1@1.0.0"], 1, "compatible cache is reused")
			libc = "musl libc"
			_, err = f.b.Get(t.Context(), "tool1")
			assert.ErrorContains(t, err, "no compatible Packslip artifact")
			assert.Equal(t, string(installationRead(t, path)), "tool1@1.0.0")
			libc = "GNU libc 2.39"
			_, err = f.b.Get(t.Context(), "tool1")
			assert.NilError(t, err)
			assert.Equal(t, f.downloads["tool1@1.0.0"], 1, "host mismatch preserves the existing cache")
			f.assertClean(t)
		})
	}
}

func TestPackslipUpgradeMarkerFailureRecovery(t *testing.T) {
	f := newInstallationFixture(t, "packslip")
	path, err := f.b.Get(t.Context(), "tool1")
	assert.NilError(t, err)
	oldConfig := installationRead(t, f.configPath)
	oldMarkerPath := filepath.Join(f.b.VersionsDir, "tool1", "1.0.0")
	oldMarker := installationRead(t, oldMarkerPath)
	oldTrust := installationRead(t, filepath.Join(f.stateDir, "trust.json"))
	blocked := filepath.Join(f.b.VersionsDir, "tool1", "2.0.0")
	installationBlock(t, blocked)
	updates, err := f.b.UpgradeOne(t.Context(), "tool1")
	assert.ErrorContains(t, err, blocked)
	assert.Equal(t, len(updates), 1)
	assert.DeepEqual(t, installationRead(t, f.configPath), oldConfig)
	assert.Equal(t, f.b.config.Bins[0].Version, "1.0.0")
	assert.DeepEqual(t, installationRead(t, oldMarkerPath), oldMarker)
	assert.Equal(t, string(installationRead(t, path)), "tool1@2.0.0")
	assert.Assert(t, !bytes.Equal(installationRead(t, filepath.Join(f.stateDir, "trust.json")), oldTrust))
	f.assertClean(t)
	assert.NilError(t, os.RemoveAll(blocked))
	// Get follows the unchanged pin and restores version 1; upgrade can then retry.
	_, err = f.b.Get(t.Context(), "tool1")
	assert.NilError(t, err)
	assert.Equal(t, string(installationRead(t, path)), "tool1@1.0.0")
	_, err = f.b.UpgradeOne(t.Context(), "tool1")
	assert.NilError(t, err)
	assert.Equal(t, string(installationRead(t, path)), "tool1@2.0.0")
	assert.Equal(t, f.b.config.Bins[0].Version, "2.0.0")
	f.assertClean(t)
}

func TestPackslipGetRejectsTrustEstablishedDuringDownload(t *testing.T) {
	f := newInstallationFixture(t, "packslip")
	f.onDownload = func() {
		r := &packslip.Resolver{StateDir: f.stateDir}
		competing := &packslip.Resolved{Identity: packslip.Identity{
			Repository: "https://github.com/example/tool1", RepositoryID: "100", OwnerID: "456",
			Signer: "https://github.com/example/tool1/.github/workflows/other.yml@refs/tags/v1.0.0",
		}}
		assert.NilError(t, r.Accept("github.com/example/tool1", competing))
	}
	_, err := f.b.Get(t.Context(), "tool1")
	assert.ErrorContains(t, err, "signing workflow changed")
	trust := installationRead(t, filepath.Join(f.stateDir, "trust.json"))
	_, err = os.Stat(filepath.Join(f.b.BinDir, "tool1"))
	assert.Assert(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(f.b.VersionsDir, "tool1", "1.0.0"))
	assert.Assert(t, os.IsNotExist(err))
	f.assertClean(t)
	f.onDownload = nil
	_, err = f.b.Get(t.Context(), "tool1")
	assert.ErrorContains(t, err, "signing workflow changed")
	assert.Equal(t, f.downloads["tool1@1.0.0"], 1)
	assert.DeepEqual(t, installationRead(t, filepath.Join(f.stateDir, "trust.json")), trust)
}

func TestUpgradeConfigurationWriteFailure(t *testing.T) {
	for _, format := range []configFormat{configFormatTOML, configFormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			f := newInstallationFixture(t, "packslip")
			if format == configFormatJSON {
				assert.NilError(t, os.Remove(f.configPath))
				f.configPath = filepath.Join(f.dir, ".bine.json")
				f.b.config.path, f.b.config.format = f.configPath, format
				assert.NilError(t, os.WriteFile(f.configPath, installationJSON(t, f.b.config), 0o600))
			}
			path, err := f.b.Get(t.Context(), "tool1")
			assert.NilError(t, err)
			oldConfig := installationRead(t, f.configPath)
			// JSON writes the existing file; TOML creates a replacement in its parent.
			t.Cleanup(func() {
				assert.NilError(t, os.Chmod(f.dir, 0o700))
				assert.NilError(t, os.Chmod(f.configPath, 0o600))
			})
			assert.NilError(t, os.Chmod(f.configPath, 0o400))
			assert.NilError(t, os.Chmod(f.dir, 0o500))
			var probe *os.File
			if format == configFormatJSON {
				probe, err = os.OpenFile(f.configPath, os.O_WRONLY, 0)
			} else {
				probe, err = os.CreateTemp(f.dir, "write-probe-*")
			}
			if err == nil {
				assert.NilError(t, probe.Close())
				t.Skip("filesystem permits writes despite permissions")
			}
			assert.Assert(t, os.IsPermission(err))
			updates, err := f.b.UpgradeOne(t.Context(), "tool1")
			assert.ErrorContains(t, err, "permission denied")
			assert.Assert(t, updates == nil)
			assert.DeepEqual(t, installationRead(t, f.configPath), oldConfig)
			assert.Equal(t, f.b.config.Bins[0].Version, "1.0.0")
			assert.Equal(t, string(installationRead(t, path)), "tool1@2.0.0")
			assert.NilError(t, os.Chmod(f.dir, 0o700))
			assert.NilError(t, os.Chmod(f.configPath, 0o600))
			_, err = f.b.Get(t.Context(), "tool1")
			assert.NilError(t, err)
			assert.Equal(t, string(installationRead(t, path)), "tool1@1.0.0")
			_, err = f.b.UpgradeOne(t.Context(), "tool1")
			assert.NilError(t, err)
			assert.Equal(t, string(installationRead(t, path)), "tool1@2.0.0")
			f.assertClean(t)
		})
	}
}

func TestUpgradePartialFailureRecovery(t *testing.T) {
	for _, second := range []string{"packslip", "recipe"} {
		t.Run("later "+second+" fails", func(t *testing.T) {
			f := newInstallationFixture(t, "packslip", second)
			assert.NilError(t, f.b.Sync(t.Context()))
			f.fail["tool2@2.0.0"] = true
			updates, err := f.b.Upgrade(t.Context())
			assert.ErrorContains(t, err, "Service Unavailable")
			assert.Equal(t, len(updates), 2)
			assert.Equal(t, string(installationRead(t, filepath.Join(f.b.BinDir, "tool1"))), "tool1@2.0.0")
			assert.Equal(t, string(installationRead(t, filepath.Join(f.b.BinDir, "tool2"))), "tool2@1.0.0")
			wantPin := "1.0.0"
			if second == "recipe" {
				wantPin = "2.0.0"
			}
			cfg, err := unmarshalTOMLConfig(installationRead(t, f.configPath))
			assert.NilError(t, err)
			for i, bin := range cfg.Bins {
				assert.Equal(t, bin.Version, wantPin)
				assert.Equal(t, f.b.config.Bins[i].Version, wantPin)
			}
			f.assertClean(t)
			f.fail["tool2@2.0.0"] = false
			updates, err = f.b.Upgrade(t.Context())
			assert.NilError(t, err)
			if second == "recipe" {
				assert.Equal(t, len(updates), 0, "retry repairs through Sync after pins advance")
				assert.Equal(t, f.downloads["tool1@2.0.0"], 1, "the successful Packslip install is reused")
			}
			for _, name := range []string{"tool1", "tool2"} {
				path, err := f.b.Get(t.Context(), name)
				assert.NilError(t, err)
				assert.Equal(t, string(installationRead(t, path)), name+"@2.0.0")
			}
			f.assertClean(t)
		})
	}
}

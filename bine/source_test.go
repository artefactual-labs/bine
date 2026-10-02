package bine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRecipeUpgradeDownloadsUpdatedTagAndAsset(t *testing.T) {
	var mu sync.Mutex
	var downloads []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			fmt.Fprint(w, `[{"tag_name":"release-2.0.0"}]`)
			return
		}
		mu.Lock()
		downloads = append(downloads, r.URL.Path)
		mu.Unlock()
		fmt.Fprint(w, r.URL.Path)
	}))
	defer server.Close()

	dir := t.TempDir()
	t.Chdir(dir)
	before := `project = "test"
[[bins]]
name = "local-tool"
version = "1.0.0" # Keep the comment.
url = "https://github.com/example/tool"
tag_pattern = "release-{version}"
asset_pattern = "{name}-{version}"
`
	before = strings.ReplaceAll(before, "https://github.com/example/tool", server.URL+"/github.com/example/tool")
	assert.NilError(t, os.WriteFile(".bine.toml", []byte(before), 0o600))
	cfg, err := loadConfig(t.Context(), sourceFactory{client: &http.Client{Transport: &mockTransport{mockServer: server}}})
	assert.NilError(t, err)
	b := &Bine{config: cfg, BinDir: filepath.Join(dir, "bin"), VersionsDir: filepath.Join(dir, "versions")}
	path, err := b.Get(t.Context(), "local-tool")
	assert.NilError(t, err)
	updates, err := b.Upgrade(t.Context())
	assert.NilError(t, err)
	assert.Equal(t, len(updates), 1)
	mu.Lock()
	actual := append([]string(nil), downloads...)
	mu.Unlock()
	assert.DeepEqual(t, actual, []string{
		"/github.com/example/tool/releases/download/release-1.0.0/local-tool-1.0.0",
		"/github.com/example/tool/releases/download/release-2.0.0/local-tool-2.0.0",
	})
	installed, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(installed), actual[1])
	updated, err := os.ReadFile(".bine.toml")
	assert.NilError(t, err)
	assert.Equal(t, string(updated), strings.Replace(before, `version = "1.0.0"`, `version = "2.0.0"`, 1))
	assert.NilError(t, b.Sync(t.Context()))
	mu.Lock()
	count := len(downloads)
	mu.Unlock()
	assert.Equal(t, count, 2, "the new marker should permit cache reuse")
}

func TestRecipeReinstallFailurePreservesPublishedBinary(t *testing.T) {
	for _, stage := range []string{"download", "marker"} {
		t.Run(stage, func(t *testing.T) {
			var mu sync.Mutex
			failDownload := false
			content := "original binary"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				fail, body := failDownload, content
				mu.Unlock()
				if fail {
					http.Error(w, "unavailable", http.StatusBadGateway)
					return
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			dir := t.TempDir()
			t.Chdir(dir)
			config := `project="test"
[[bins]]
name="tool"
version="1.0.0"
url="https://github.com/example/tool"
asset_pattern="tool-{version}"
`
			config = strings.ReplaceAll(config, "https://github.com/example/tool", server.URL+"/github.com/example/tool")
			assert.NilError(t, os.WriteFile(".bine.toml", []byte(config), 0o600))
			cfg, err := loadConfig(t.Context(), sourceFactory{client: &http.Client{Transport: &mockTransport{mockServer: server}}})
			assert.NilError(t, err)
			b := &Bine{config: cfg, BinDir: filepath.Join(dir, "bin"), VersionsDir: filepath.Join(dir, "versions")}
			path, err := b.Get(t.Context(), "tool")
			assert.NilError(t, err)
			marker := filepath.Join(b.VersionsDir, "tool", "1.0.0")
			oldMarker, err := os.ReadFile(marker)
			assert.NilError(t, err)
			mu.Lock()
			content = "replacement binary"
			failDownload = stage == "download"
			mu.Unlock()
			want := "original binary"
			if stage == "marker" {
				// Make marker publication fail after executable replacement.
				assert.NilError(t, os.Remove(marker))
				assert.NilError(t, os.Mkdir(marker, 0o750))
				assert.NilError(t, os.WriteFile(filepath.Join(marker, "keep"), nil, 0o600))
				want = "replacement binary"
			}
			_, err = b.GetForce(t.Context(), "tool")
			assert.Assert(t, err != nil)
			installed, err := os.ReadFile(path)
			assert.NilError(t, err)
			assert.Equal(t, string(installed), want)
			if stage == "download" {
				currentMarker, err := os.ReadFile(marker)
				assert.NilError(t, err)
				assert.DeepEqual(t, currentMarker, oldMarker)
			} else {
				assert.NilError(t, os.RemoveAll(marker))
				_, err := b.Get(t.Context(), "tool")
				assert.NilError(t, err, "a missing marker should trigger a recoverable reinstall")
			}
		})
	}
}

func TestGetForceGoLatestUsesRecoveredVersion(t *testing.T) {
	injectFakeExec(t, "TestHelperProcessWithSuccess")
	dir := t.TempDir()
	t.Chdir(dir)
	assert.NilError(t, os.WriteFile(".bine.toml", []byte(`project="test"
[[bins]]
name="local-tool"
version="latest"
go_package="example.com/tool/v2"
`), 0o600))
	requested := filepath.Join(dir, "requested-package")
	t.Setenv("BINE_HELPER_REQUESTED_PACKAGE", requested)
	b, err := NewWithOptions(WithCacheDir(filepath.Join(dir, "cache")))
	assert.NilError(t, err)
	assert.NilError(t, os.MkdirAll(b.BinDir, 0o750))
	assert.NilError(t, os.WriteFile(filepath.Join(b.BinDir, "local-tool"), []byte("original"), 0o755))
	markerDir := filepath.Join(b.VersionsDir, "local-tool")
	assert.NilError(t, os.MkdirAll(markerDir, 0o750))
	sum := sha256.Sum256([]byte("original"))
	marker := `{"checksum":{"algorithm":"SHA-256","value":"` + hex.EncodeToString(sum[:]) + `"},"resolved_version":"1.2.3"}`
	assert.NilError(t, os.WriteFile(filepath.Join(markerDir, "latest"), []byte(marker), 0o640))
	path, err := b.GetForce(t.Context(), "local-tool")
	assert.NilError(t, err)
	assert.Equal(t, filepath.Base(path), "local-tool")
	packageName, err := os.ReadFile(requested)
	assert.NilError(t, err)
	assert.Equal(t, string(packageName), "example.com/tool/v2@v1.2.3")
	data, err := os.ReadFile(filepath.Join(markerDir, "latest"))
	assert.NilError(t, err)
	var persisted struct {
		ResolvedVersion string `json:"resolved_version"`
	}
	assert.NilError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, persisted.ResolvedVersion, "1.2.3")
	_, err = os.Stat(filepath.Join(markerDir, "1.2.3"))
	assert.Assert(t, os.IsNotExist(err))
	config, err := os.ReadFile(".bine.toml")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(config), `version="latest"`))
}

func TestSourceConstructionAvoidsUnneededHostAccess(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", "")
	assert.NilError(t, os.WriteFile(".bine.toml", []byte(`project="test"
[[bins]]
name="tool"
version="1.0.0"
go_package="example.com/tool"
url="https://github.com/example/tool"
`), 0o600))
	b, err := NewWithOptions(WithCacheDir(filepath.Join(dir, "cache")))
	assert.NilError(t, err)
	items, err := b.List(t.Context(), false, false)
	assert.NilError(t, err)
	assert.Equal(t, len(items), 1)
}

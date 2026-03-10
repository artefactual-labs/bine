package bine

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestBinVersions(t *testing.T) {
	tests := []struct {
		version    string // Provided by the user (we want to be flexible).
		canonical  string
		usable     string
		unprefixed string
	}{
		{"1.2.3", "v1.2.3", "v1.2.3", "1.2.3"},
		{"v1.2.3", "v1.2.3", "v1.2.3", "1.2.3"},
		{"jq-1.7", "", "jq-1.7", "jq-1.7"},
		{"jq-1.7.1", "", "jq-1.7.1", "jq-1.7.1"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Version %q", test.version), func(t *testing.T) {
			b := bin{Version: test.version}
			assert.Equal(t, b.canonicalVersion(), test.canonical)
			assert.Equal(t, b.usableVersion(), test.usable)
			assert.Equal(t, b.unprefixedVersion(), test.unprefixed)
		})
	}
}

func TestBinTag(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		tagPattern  string
		expectedTag string
	}{
		{"Default pattern", "1.2.3", "", "v1.2.3"},
		{"uv style: no prefix", "0.7.11", "{version}", "0.7.11"},
		{"jq style: prefix", "1.8.0", "jq-{version}", "jq-1.8.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := bin{
				Version:    test.version,
				TagPattern: test.tagPattern,
			}
			assert.Equal(t, b.tag(), test.expectedTag)
		})
	}
}

func TestGitHubProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log(r.URL.Path)
		if strings.Contains(r.URL.Path, "/repos/sevein/perpignan/releases") {
			releases := []githubRelease{
				{TagName: "v1.0.2-rc.1", Prerelease: true},
				{TagName: "v1.0.1"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(releases)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &mockTransport{
			mockServer: server,
		},
	}

	provider := &githubProvider{
		client: client,
		token:  "test-token",
	}

	t.Run("downloadURL", func(t *testing.T) {
		bin := &bin{
			Name:    "perpignan",
			Version: "1.0.0",
			URL:     "https://github.com/sevein/perpignan",
			asset:   "perpignan-linux-amd64",
		}

		downloadURL, err := provider.downloadURL(bin)
		assert.NilError(t, err)
		assert.Equal(t, downloadURL, "https://github.com/sevein/perpignan/releases/download/v1.0.0/perpignan-linux-amd64")
	})

	t.Run("latestVersion", func(t *testing.T) {
		ctx := context.Background()
		bin := &bin{
			Name:    "perpignan",
			Version: "1.0.0",
			URL:     "https://github.com/sevein/perpignan",
		}

		latestVersion, err := provider.latestVersion(ctx, bin)
		assert.NilError(t, err)
		assert.Equal(t, latestVersion, "1.0.1")
	})
}

func TestArigaProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/ariga/atlas/releases") {
			releases := []githubRelease{
				{TagName: "v0.35.0-rc.2", Prerelease: true},
				{TagName: "v0.34.0"},
				{TagName: "v0.33.0"},
				{TagName: "v0.32.0"},
				{TagName: "v0.31.0"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(releases)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &mockTransport{
			mockServer: server,
		},
	}

	provider := &arigaProvider{
		client: client,
		token:  "test-token",
	}

	t.Run("downloadURL", func(t *testing.T) {
		bin := &bin{
			Name:    "atlas",
			Version: "0.31.0",
			URL:     "https://release.ariga.io/atlas",
			asset:   "atlas-linux-amd64",
		}

		downloadURL, err := provider.downloadURL(bin)
		assert.NilError(t, err)
		assert.Equal(t, downloadURL, "https://release.ariga.io/atlas/atlas-linux-amd64")
	})

	t.Run("latestVersion", func(t *testing.T) {
		ctx := context.Background()
		bin := &bin{
			Name:    "atlas",
			Version: "0.31.0",
			URL:     "https://release.ariga.io/atlas",
		}

		latestVersion, err := provider.latestVersion(ctx, bin)
		assert.NilError(t, err)
		assert.Equal(t, latestVersion, "0.34.0")
	})
}

func TestVersionMarkerChecksumMatches(t *testing.T) {
	c := versionMarkerChecksum{
		Algorithm: crypto.SHA256.String(),
		Value:     "expected",
	}

	assert.Assert(t, c.Matches("expected"))
	assert.Assert(t, !c.Matches("different"))
}

func TestVersionMarkerDocumentResolvedVersion(t *testing.T) {
	t.Run("Serializes resolved version", func(t *testing.T) {
		doc := versionMarkerDocument{
			Checksum: versionMarkerChecksum{
				Algorithm: crypto.SHA256.String(),
				Value:     "abc123",
			},
			ResolvedVersion: "1.2.3",
		}
		data, err := json.Marshal(doc)
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(data), `"resolved_version":"1.2.3"`))
	})

	t.Run("Omits resolved_version when empty", func(t *testing.T) {
		doc := versionMarkerDocument{
			Checksum: versionMarkerChecksum{
				Algorithm: crypto.SHA256.String(),
				Value:     "abc123",
			},
		}
		data, err := json.Marshal(doc)
		assert.NilError(t, err)
		assert.Assert(t, !strings.Contains(string(data), "resolved_version"))
	})
}

func TestBinIsLatest(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		goPackage string
		want      bool
	}{
		{"empty version for go package", "", "github.com/foo/bar", true},
		{"latest for go package", "latest", "github.com/foo/bar", true},
		{"LATEST for go package (case-insensitive)", "LATEST", "github.com/foo/bar", true},
		{"pinned version for go package", "v1.2.3", "github.com/foo/bar", false},
		{"empty version without go package", "", "", false},
		{"latest without go package", "latest", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bin{Version: tt.version, GoPackage: tt.goPackage}
			assert.Equal(t, b.isLatest(), tt.want)
		})
	}
}

func TestBinMarkerVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		goPackage string
		want      string
	}{
		{"pinned semver", "v1.2.3", "", "v1.2.3"},
		{"unprefixed semver", "1.2.3", "", "1.2.3"},
		{"empty version for go package", "", "github.com/foo/bar", "latest"},
		{"latest for go package", "latest", "github.com/foo/bar", "latest"},
		{"LATEST for go package (case-insensitive)", "LATEST", "github.com/foo/bar", "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bin{Version: tt.version, GoPackage: tt.goPackage}
			assert.Equal(t, b.markerVersion(), tt.want)
		})
	}
}

func TestCheckOutdated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/foo/bar/releases") {
			releases := []githubRelease{
				{TagName: "v2.0.0"},
				{TagName: "v1.0.0"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(releases)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &mockTransport{mockServer: server},
	}

	makeBin := func(version string) *bin {
		b := &bin{
			Name:    "bar",
			Version: version,
			URL:     "https://github.com/foo/bar",
		}
		b.provider = &githubProvider{client: client}
		return b
	}

	ctx := context.Background()

	t.Run("pinned outdated version", func(t *testing.T) {
		b := makeBin("1.0.0")
		outdated, latest, err := b.checkOutdated(ctx, "")
		assert.NilError(t, err)
		assert.Assert(t, outdated)
		assert.Equal(t, latest, "2.0.0")
	})

	t.Run("pinned up-to-date version", func(t *testing.T) {
		b := makeBin("2.0.0")
		outdated, _, err := b.checkOutdated(ctx, "")
		assert.NilError(t, err)
		assert.Assert(t, !outdated)
	})

	t.Run("latest bin with outdated resolved version", func(t *testing.T) {
		b := makeBin("latest")
		b.GoPackage = "github.com/foo/bar"
		outdated, latest, err := b.checkOutdated(ctx, "1.0.0")
		assert.NilError(t, err)
		assert.Assert(t, outdated)
		assert.Equal(t, latest, "2.0.0")
	})

	t.Run("latest bin with up-to-date resolved version", func(t *testing.T) {
		b := makeBin("latest")
		b.GoPackage = "github.com/foo/bar"
		outdated, _, err := b.checkOutdated(ctx, "2.0.0")
		assert.NilError(t, err)
		assert.Assert(t, !outdated)
	})

	t.Run("latest bin without resolved version returns error", func(t *testing.T) {
		b := makeBin("latest")
		b.GoPackage = "github.com/foo/bar"
		_, _, err := b.checkOutdated(ctx, "")
		assert.ErrorContains(t, err, "has no resolved version to compare")
	})

	t.Run("empty version without resolved version returns error", func(t *testing.T) {
		b := makeBin("")
		b.GoPackage = "github.com/foo/bar"
		_, _, err := b.checkOutdated(ctx, "")
		assert.ErrorContains(t, err, "has no resolved version to compare")
	})
}

// mockTransport redirects GitHub API calls to our test server
type mockTransport struct {
	mockServer *httptest.Server
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect GitHub API calls to our mock server
	if req.URL.Host == "api.github.com" {
		// Parse the mock server URL and replace the request URL
		mockURL, err := url.Parse(t.mockServer.URL)
		if err != nil {
			return nil, err
		}

		// Keep the original path but change the host
		req.URL.Scheme = mockURL.Scheme
		req.URL.Host = mockURL.Host
	}

	// Use the default transport for the actual request
	return http.DefaultTransport.RoundTrip(req)
}

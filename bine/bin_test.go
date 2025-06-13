package bine

import (
	"context"
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

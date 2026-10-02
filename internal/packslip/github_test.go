package packslip

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gotest.tools/v3/assert"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func publishedResolver(t *testing.T) (*Resolver, Request, func(string, string)) {
	t.Helper()
	data := fixture(t, "hk-v2.4.0.sigstore.json")
	root := testRoot(t)
	_, id, err := VerifyBundle(data, root)
	assert.NilError(t, err)
	idNumber, err := strconv.ParseInt(id.RepositoryID, 10, 64)
	assert.NilError(t, err)
	repoData, err := json.Marshal(map[string]any{"id": idNumber, "owner": map[string]int{"id": 216188}, "full_name": "jdx/hk", "default_branch": "main"})
	assert.NilError(t, err)
	responses := map[string]string{
		"https://api.github.com/repos/jdx/hk":                                       string(repoData),
		"https://api.github.com/repos/jdx/hk/releases?per_page=100&page=1":          `[{"tag_name":"v2.4.0","assets":[{"name":"packslip.sigstore.json","browser_download_url":"https://github.com/jdx/hk/releases/download/v2.4.0/packslip.sigstore.json"}]}]`,
		"https://github.com/jdx/hk/releases/download/v2.4.0/packslip.sigstore.json": string(data),
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			assert.Equal(t, req.Header.Get("Authorization"), "Bearer test-token")
		} else {
			assert.Equal(t, req.Header.Get("Authorization"), "")
		}
		body, ok := responses[req.URL.String()]
		status := http.StatusOK
		if !ok {
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})}
	r := &Resolver{Client: client, Token: "test-token", StateDir: t.TempDir(), Host: Host{"darwin", "aarch64", ""}, VerifyBundle: func(_ context.Context, data []byte) ([]byte, Identity, error) { return VerifyBundle(data, root) }}
	return r, Request{Project: "github.com/jdx/hk", Version: "2.4.0", Name: "hk"}, func(url, body string) { responses[url] = body }
}

func TestResolvePublishedGitHubRelease(t *testing.T) {
	r, request, _ := publishedResolver(t)
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	assert.Equal(t, result.Artifact.Name, "hk-aarch64-apple-darwin.tar.gz")
	assert.Equal(t, result.Executable.Name, "hk")
	assert.Equal(t, result.Version, "2.4.0")
	assert.Equal(t, len(result.ManifestSHA256), 64)
	assert.NilError(t, r.Accept(request.Project, result))
	ok, err := r.Accepted(request.Project, result)
	assert.NilError(t, err)
	assert.Assert(t, ok)
	latest, err := r.Latest(t.Context(), request)
	assert.NilError(t, err)
	assert.Equal(t, latest, "2.4.0")
}

func TestResolveRenamedRepositoryTags(t *testing.T) {
	s := sampleStatement()
	s.Predicate.Version = "1.2.3"
	s.Predicate.Artifacts[0].URL = "https://github.com/example/tool/releases/download/tool-1.2.3/tool.tar.gz"
	payload, err := json.Marshal(s)
	assert.NilError(t, err)
	data, err := json.Marshal(map[string]any{"dsseEnvelope": map[string][]byte{"payload": payload}})
	assert.NilError(t, err)
	fullName := "example/tool"
	r := &Resolver{StateDir: t.TempDir(), Host: Host{"darwin", "aarch64", ""}}
	r.VerifyBundle = func(_ context.Context, bundle []byte) ([]byte, Identity, error) {
		assert.DeepEqual(t, bundle, data)
		return payload, Identity{Repository: "https://github.com/example/tool", RepositoryID: "123", OwnerID: "456", Signer: s.Predicate.Identity.KeyID}, nil
	}
	r.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, status := "", http.StatusOK
		switch {
		case req.URL.Path == "/repos/example/tool" || req.URL.Path == "/repos/example/new-tool":
			metadata, err := json.Marshal(map[string]any{"id": 123, "owner": map[string]int{"id": 456}, "full_name": fullName, "default_branch": "main"})
			assert.NilError(t, err)
			body = string(metadata)
		case strings.HasSuffix(req.URL.Path, "/releases"):
			body = `[{"tag_name":"tool-1.2.3","assets":[{"name":"packslip.sigstore.json","browser_download_url":"https://github.com/example/tool/releases/download/tool-1.2.3/packslip.sigstore.json"}]}]`
		case req.URL.String() == "https://github.com/example/tool/releases/download/tool-1.2.3/packslip.sigstore.json":
			body = string(data)
		default:
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	request := Request{Project: "github.com/example/tool", Version: "1.2.3", Name: "tool"}
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	assert.NilError(t, r.Accept(request.Project, result))
	fullName = "example/new-tool"
	for _, project := range []string{"github.com/example/tool", "github.com/example/new-tool"} {
		request.Project = project
		result, err = r.Resolve(t.Context(), request)
		assert.NilError(t, err)
		assert.Equal(t, result.Version, "1.2.3")
		latest, err := r.Latest(t.Context(), request)
		assert.NilError(t, err)
		assert.Equal(t, latest, "1.2.3")
	}
}

func TestDiscoveryRefusesPolicyAndIdentityFailures(t *testing.T) {
	for _, kind := range []string{"signed list", "version mismatch", "different repository", "missing bundle", "HTTP bundle", "missing project", "duplicate manifest", "owner redirect"} {
		t.Run(kind, func(t *testing.T) {
			r, request, set := publishedResolver(t)
			want := ""
			switch kind {
			case "signed list":
				set("https://api.github.com/repos/jdx/hk/contents/.well-known/packslip.json?ref=main", `{}`)
				want = "supplementary signed"
			case "version mismatch":
				request.Version = "2.4.1"
				set("https://api.github.com/repos/jdx/hk/releases?per_page=100&page=1", `[{"tag_name":"v2.4.1","assets":[{"name":"packslip.sigstore.json","browser_download_url":"https://github.com/jdx/hk/releases/download/v2.4.0/packslip.sigstore.json"}]}]`)
				want = "version or source tag"
			case "different repository":
				set("https://api.github.com/repos/jdx/hk", `{"id":99,"owner":{"id":216188},"full_name":"jdx/hk","default_branch":"main"}`)
				want = "does not match"
			case "missing bundle":
				set("https://api.github.com/repos/jdx/hk/releases?per_page=100&page=1", `[{"tag_name":"v2.4.0","assets":[{"name":"packslip.sigstore.json","browser_download_url":"https://github.com/missing"}]}]`)
				want = "bundle is missing"
			case "HTTP bundle":
				set("https://api.github.com/repos/jdx/hk/releases?per_page=100&page=1", `[{"tag_name":"v2.4.0","assets":[{"name":"packslip.sigstore.json","browser_download_url":"http://github.com/bundle"}]}]`)
				want = "HTTPS"
			case "missing project":
				request.Project = "github.com/jdx/hk/other"
				want = "not found"
			case "duplicate manifest":
				set("https://api.github.com/repos/jdx/hk/releases?per_page=100&page=1", `[{"tag_name":"v2.4.0","assets":[{"name":"packslip.sigstore.json","browser_download_url":"https://github.com/jdx/hk/releases/download/v2.4.0/packslip.sigstore.json"},{"name":"packslip.other.sigstore.json","browser_download_url":"https://github.com/jdx/hk/releases/download/v2.4.0/packslip.sigstore.json"}]}]`)
				want = "multiple Packslip"
			case "owner redirect":
				set("https://api.github.com/repos/jdx/hk", `{"id":99,"owner":{"id":42},"full_name":"other/hk","default_branch":"main"}`)
				want = "another owner"
			}
			_, err := r.Resolve(t.Context(), request)
			assert.ErrorContains(t, err, want)
		})
	}
}

func TestTrustSurvivesReinstallAndRejectsChanges(t *testing.T) {
	r, request, _ := publishedResolver(t)
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	assert.NilError(t, r.Accept(request.Project, result))
	for _, kind := range []string{"repository", "owner", "workflow", "provenance", "relaxed platform"} {
		t.Run(kind, func(t *testing.T) {
			changed := *result
			switch kind {
			case "repository":
				changed.Identity.RepositoryID = "new"
			case "owner":
				changed.Identity.OwnerID = "new"
			case "workflow":
				changed.Identity.Signer = strings.Replace(changed.Identity.Signer, "release.yml", "other.yml", 1)
			case "provenance":
				changed.Artifact.Provenance = nil
			case "relaxed platform":
				changed.Artifact.Provenance = nil
				changed.Artifact.OS = ""
				changed.Artifact.Arch = ""
				changed.Artifact.Libc = ""
			}
			accepted, err := r.Accepted(request.Project, &changed)
			assert.Assert(t, err != nil)
			assert.Assert(t, !accepted)
			assert.Assert(t, r.Accept(request.Project, &changed) != nil)
		})
	}
	newTag := *result
	newTag.Identity.Signer = strings.Replace(newTag.Identity.Signer, "v2.4.0", "v2.4.1", 1)
	assert.NilError(t, r.Accept(request.Project, &newTag))
	// A new Resolver (another invocation) must read the same continuity pins.
	other := &Resolver{StateDir: r.StateDir}
	ok, err := other.Accepted(request.Project, result)
	assert.NilError(t, err)
	assert.Assert(t, ok)
	renamed := *result
	renamed.Identity.Repository = "https://github.com/jdx/new-hk"
	renamed.Identity.Signer = strings.Replace(renamed.Identity.Signer, "jdx/hk/", "jdx/new-hk/", 1)
	assert.NilError(t, r.Accept("github.com/jdx/new-hk", &renamed))
	renamed.Identity.Signer = strings.Replace(renamed.Identity.Signer, "release.yml", "other.yml", 1)
	assert.Assert(t, r.Accept("github.com/jdx/new-hk", &renamed) != nil)
}

func TestConcurrentFirstUseCannotReplaceSigner(t *testing.T) {
	dir := t.TempDir()
	ready := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, workflow := range []string{"release.yml", "different.yml"} {
		wg.Go(func() {
			r := &Resolver{StateDir: dir}
			result := &Resolved{Identity: Identity{Repository: "https://github.com/example/tool", RepositoryID: "123", OwnerID: "456", Signer: "https://github.com/example/tool/.github/workflows/" + workflow + "@refs/tags/v1.2.3"}}
			<-ready
			results <- r.Accept("github.com/example/tool", result)
		})
	}
	close(ready)
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			assert.ErrorContains(t, err, "signing workflow changed")
		}
	}
	assert.Equal(t, accepted, 1)
}

func TestMetadataRedirectsDoNotLeakToken(t *testing.T) {
	for _, target := range []string{"https://sub.api.github.com/bundle", "http://api.github.com/bundle"} {
		t.Run(target, func(t *testing.T) {
			requests := 0
			r := &Resolver{Token: "secret", Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					assert.Equal(t, req.Header.Get("Authorization"), "Bearer secret")
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {target}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
				}
				assert.Equal(t, req.Header.Get("Authorization"), "")
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
			})}}
			_, _, err := r.get(t.Context(), "https://api.github.com/start")
			if strings.HasPrefix(target, "http:") {
				assert.ErrorContains(t, err, "unsafe")
				assert.Equal(t, requests, 1)
			} else {
				assert.NilError(t, err)
				assert.Equal(t, requests, 2)
			}
		})
	}
}

package bine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

type bin struct {
	Name    string `json:"name"`
	Version string `json:"version"`

	// Fields for asset-based downloads.
	URL          string `json:"url,omitempty"`
	AssetPattern string `json:"asset_pattern,omitempty"`

	// Field for go-based installs.
	GoPackage string `json:"go_package,omitempty"`

	// asset is computed by the namer when the config is loaded.
	asset string

	provider binProvider
}

func (b bin) goPkg() bool {
	return b.GoPackage != ""
}

// canonicalVersion returns the canonical formatting of the semver version.
// Useful in contexts when semver-compliant versions MUST be present.
func (b bin) canonicalVersion() string {
	if b.Version == "" {
		return ""
	}
	return semver.Canonical("v" + strings.TrimPrefix(b.Version, "v"))
}

// unprefixedVersion returns the version without the "v" prefix.
// Useful in contexts when semver-compliant versions MAY be present.
func (b bin) unprefixedVersion() string {
	return strings.TrimPrefix(b.usableVersion(), "v")
}

// usableVersion falls back to the original version if semver is not available.
// Useful in contexts where semver is not required, e.g. during downloads.
func (b bin) usableVersion() string {
	version := b.canonicalVersion()
	if version == "" {
		return b.Version
	}
	return version
}

func (b *bin) loadProvider(client *http.Client, ghAPIToken string) error {
	if b.provider != nil {
		return nil
	}

	switch {
	case b.goPkg():
		b.provider = &goProvider{client: client}
	case strings.Contains(b.URL, "github.com"):
		b.provider = &githubProvider{client: client, token: ghAPIToken}
	case strings.Contains(b.URL, "release.ariga.io"):
		b.provider = &arigaProvider{client: client, token: ghAPIToken}
	default:
		return fmt.Errorf("unsupported binary provider for %q (%s)", b.Name, b.URL)
	}

	return nil
}

// checkOutdated checks if the binary is outdated by comparing its version with
// the latest version available.
func (b *bin) checkOutdated(ctx context.Context) (bool, string, error) {
	if b.Version == "" {
		return false, "", fmt.Errorf("binary %q has no version specified", b.Name)
	}

	latestVersion, err := b.provider.latestVersion(ctx, b)
	if err != nil {
		return false, "", fmt.Errorf("check failed for binary %q: %v", b.Name, err)
	}

	// Compare versions using semver.
	current := b.canonicalVersion()
	latest := semver.Canonical("v" + strings.TrimPrefix(latestVersion, "v"))
	if latest == "" {
		return false, "", fmt.Errorf("invalid semver for latest version %q of %s", latest, b.Name)
	}

	isOutdated := semver.Compare(current, latest) < 0

	return isOutdated, latestVersion, nil
}

type binProvider interface {
	downloadURL(bin *bin) (string, error)
	latestVersion(ctx context.Context, bin *bin) (string, error)
}

type goProvider struct {
	client *http.Client
}

var _ binProvider = &goProvider{}

func (p *goProvider) downloadURL(b *bin) (string, error) {
	return "", nil // Unused.
}

// latestVersion retrieves the latest version for a Go package binary.
//
// It uses the pkg.go.dev website to find the latest version of the package.
//
// TODO: investigate if we can use the Go module proxy instead, e.g. see how
// github.com/icholy/gomajor does it. But we may also need to extract the path
// of a module given something like "goa.design/goa/v3/cmd/goa"?
func (p *goProvider) latestVersion(ctx context.Context, bin *bin) (string, error) {
	pkgPath := bin.GoPackage
	url := fmt.Sprintf("https://pkg.go.dev/%s?tab=versions", pkgPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pkg.go.dev returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %v", err)
	}

	// Extract version from HTML using regex to find the first js-versionLink.
	re := regexp.MustCompile(`<a class="js-versionLink"[^>]*>([^<]+)</a>`)
	matches := re.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("extract version: no match found in HTML")
	}

	latestVersion := string(matches[1])
	latestVersion = strings.TrimPrefix(latestVersion, "v")

	return latestVersion, nil
}

type githubProvider struct {
	client *http.Client
	token  string
}

var _ binProvider = &githubProvider{}

func (p *githubProvider) downloadURL(b *bin) (string, error) {
	return fmt.Sprintf("%s/releases/download/%s/%s", b.URL, b.usableVersion(), b.asset), nil
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

func (p *githubProvider) latestVersion(ctx context.Context, bin *bin) (string, error) {
	u, err := url.Parse(bin.URL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %v", err)
	}

	// Extract owner and repo from the path.
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", errors.New("could not extract owner/repo")
	}
	owner, repo := parts[0], parts[1]

	// GitHub API endpoint for releases.
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if p.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.token))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("failed to decode GitHub API response: %v", err)
	}

	// Find the latest valid semver tag among the releases, skipping prereleases.
	var latestSemver string
	for _, release := range releases {
		if release.Prerelease {
			continue
		}
		tag := release.TagName
		canonicalTag := semver.Canonical(tag)
		if canonicalTag == "" {
			continue
		}
		if latestSemver == "" || semver.Compare(canonicalTag, latestSemver) > 0 {
			latestSemver = canonicalTag
		}
	}

	if latestSemver == "" {
		return "", errors.New("no valid non-prerelease semver tags found in GitHub releases")
	}

	return strings.TrimPrefix(latestSemver, "v"), nil
}

type arigaProvider struct {
	client *http.Client
	token  string
}

var _ binProvider = &arigaProvider{}

func (p *arigaProvider) downloadURL(b *bin) (string, error) {
	parsedURL, err := url.Parse(b.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", b.URL, err)
	}

	downloadURL := parsedURL.JoinPath(b.asset).String()

	return downloadURL, nil
}

func (p *arigaProvider) latestVersion(ctx context.Context, bin *bin) (string, error) {
	// TODO: this is basically a copy of the GitHub provider.
	owner, repo := "ariga", "atlas"

	// GitHub API endpoint for releases.
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if p.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.token))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("failed to decode GitHub API response: %v", err)
	}

	// Find the latest valid semver tag among the releases.
	var latestSemver string
	for _, release := range releases {
		tag := release.TagName
		canonicalTag := semver.Canonical(tag)
		if canonicalTag == "" {
			continue
		}
		if latestSemver == "" || semver.Compare(canonicalTag, latestSemver) > 0 {
			latestSemver = canonicalTag
		}
	}

	if latestSemver == "" {
		return "", errors.New("no valid semver tags found in GitHub releases")
	}

	return strings.TrimPrefix(latestSemver, "v"), nil
}

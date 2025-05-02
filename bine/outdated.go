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

const userAgent = "bine"

// checkOutdated checks if the binary is outdated by comparing its version with
// the latest version available.
//
// TODO: manage checks using providers, e.g. GitHub, Go modules, etc.
func checkOutdated(ctx context.Context, bin *bin, httpClient *http.Client) (bool, string, error) {
	if bin.Version == "" {
		return false, "", fmt.Errorf("binary %q has no version specified", bin.Name)
	}

	var latestVersion string
	var err error
	if bin.goPkg() {
		latestVersion, err = checkGoOutdated(ctx, httpClient, bin)
	} else if strings.Contains(bin.URL, "github.com") {
		latestVersion, err = checkGitHubOutdated(ctx, httpClient, bin)
	} else {
		return false, "", fmt.Errorf("check failed for binary %q: unsupported binary provider", bin.Name)
	}
	if err != nil {
		return false, "", fmt.Errorf("check failed for binary %q: %v", bin.Name, err)
	}

	// Compare versions using semver.
	current := bin.canonicalVersion()
	latest := semver.Canonical("v" + strings.TrimPrefix(latestVersion, "v"))
	if latest == "" {
		return false, "", fmt.Errorf("invalid semver for latest version %q of %s", latest, bin.Name)
	}

	isOutdated := semver.Compare(current, latest) < 0

	return isOutdated, latestVersion, nil
}

// checkGoOutdated retrieves the latest version for a Go package binary.
//
// It uses the pkg.go.dev website to find the latest version of the package.
//
// TODO: investigate if we can use the Go module proxy instead, e.g. see how
// github.com/icholy/gomajor does it. But we may also need to extract the path
// of a module given something like "goa.design/goa/v3/cmd/goa"?
func checkGoOutdated(ctx context.Context, httpClient *http.Client, bin *bin) (string, error) {
	pkgPath := bin.GoPackage
	url := fmt.Sprintf("https://pkg.go.dev/%s?tab=versions", pkgPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
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

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// checkGitHubOutdated retrieves the latest semver version from GitHub releases.
func checkGitHubOutdated(ctx context.Context, client *http.Client, bin *bin) (string, error) {
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

	resp, err := client.Do(req)
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

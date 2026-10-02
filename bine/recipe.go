package bine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

type recipeProvider interface {
	downloadURL(b *bin, asset string) (string, error)
	latestVersion(ctx context.Context, b *bin) (string, error)
}

type recipeSource struct {
	client   *http.Client
	provider recipeProvider
	namer    *namer
}

func (s *recipeSource) downloadURL(b *bin) (string, error) {
	return s.provider.downloadURL(b, s.namer.asset(b))
}

func (s *recipeSource) install(ctx context.Context, request installRequest, target string) (installResult, error) {
	b := request.effectiveBin()
	url, err := s.downloadURL(b)
	if err == nil {
		err = installRecipe(ctx, s.client, url, target)
	} else {
		err = fmt.Errorf("failed to generate download URL: %w", err)
	}
	if err != nil {
		return installResult{}, fmt.Errorf("failed to install binary: %w", err)
	}
	return installResult{}, nil
}

func (s *recipeSource) latestVersion(ctx context.Context, b *bin) (string, error) {
	return s.provider.latestVersion(ctx, b)
}

func (s *recipeSource) validateMarker(_ context.Context, _ *bin, marker *versionMarkerDocument) (bool, error) {
	return marker.Packslip == nil, nil
}

type githubProvider struct {
	client *http.Client
	token  string
}

var _ recipeProvider = &githubProvider{}

func (p *githubProvider) downloadURL(b *bin, asset string) (string, error) {
	return fmt.Sprintf("%s/releases/download/%s/%s", b.URL, b.tag(), asset), nil
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

	return ghLatestVersion(ctx, p.client, bin, p.token, owner, repo)
}

type arigaProvider struct {
	client *http.Client
	token  string
}

var _ recipeProvider = &arigaProvider{}

func (p *arigaProvider) downloadURL(b *bin, asset string) (string, error) {
	parsedURL, err := url.Parse(b.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", b.URL, err)
	}

	downloadURL := parsedURL.JoinPath(asset).String()

	return downloadURL, nil
}

func (p *arigaProvider) latestVersion(ctx context.Context, bin *bin) (string, error) {
	return ghLatestVersion(ctx, p.client, bin, p.token, "ariga", "atlas")
}

func ghLatestVersion(ctx context.Context, client *http.Client, bin *bin, token, owner, repo string) (string, error) {
	// GitHub API endpoint for releases.
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", githubAPIStatusError(resp)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("failed to decode GitHub API response: %v", err)
	}

	// Find the latest valid semver version among the releases, skipping prereleases.
	var latestSemver string
	var latestVersion string
	for _, release := range releases {
		if release.Prerelease {
			continue
		}

		// Extract version from tag using the configured tag pattern.
		extractedVersion, matched := extractVersionFromTag(bin, release.TagName)
		if !matched {
			continue
		}

		// Validate that the extracted version is a valid semver.
		canonicalVersion := semver.Canonical("v" + strings.TrimPrefix(extractedVersion, "v"))
		if canonicalVersion == "" {
			continue
		}

		if latestSemver == "" || semver.Compare(canonicalVersion, latestSemver) > 0 {
			latestSemver = canonicalVersion
			latestVersion = extractedVersion
		}
	}

	if latestVersion == "" {
		return "", errors.New("no valid non-prerelease semver tags found in GitHub releases matching tag pattern")
	}

	return strings.TrimPrefix(latestVersion, "v"), nil
}

// extractVersionFromTag extracts a version from a tag name using the binary's
// tag pattern.
func extractVersionFromTag(bin *bin, tagName string) (string, bool) {
	// Escape special regex characters and create capture group for version.
	regexPattern := regexp.QuoteMeta(bin.tagPattern())
	regexPattern = strings.ReplaceAll(regexPattern, "\\{version\\}", "(.+)")
	regexPattern = strings.ReplaceAll(regexPattern, "\\{name\\}", regexp.QuoteMeta(bin.Name))
	regexPattern = "^" + regexPattern + "$"

	tagRegex, err := regexp.Compile(regexPattern)
	if err != nil {
		return "", false
	}

	matches := tagRegex.FindStringSubmatch(tagName)
	if len(matches) < 2 {
		return "", false
	}

	return matches[1], true
}

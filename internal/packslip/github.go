package packslip

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

const maxMetadata = 8 << 20

type Request struct {
	Project string `json:"project"`
	Version string `json:"version"`
	Command string `json:"command,omitempty"`
	Name    string `json:"name"`
	Variant string `json:"variant,omitempty"`
}

type Resolved struct {
	Project        string            `json:"project"`
	Version        string            `json:"version"`
	ManifestSHA256 string            `json:"manifest_sha256"`
	Artifact       Artifact          `json:"artifact"`
	Executable     Executable        `json:"executable"`
	Digests        map[string]string `json:"digests"`
	Identity       Identity          `json:"identity"`
}

type Resolver struct {
	Client   *http.Client
	Token    string
	StateDir string
	Host     Host
	// VerifyBundle supplies bundle verification for offline protocol and lifecycle
	// tests. When nil, verification uses the production Sigstore TUF roots.
	VerifyBundle func(context.Context, []byte) ([]byte, Identity, error)
}

type repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		ID int64 `json:"id"`
	} `json:"owner"`
	aliases []string
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type githubRelease struct {
	Tag    string         `json:"tag_name"`
	Draft  bool           `json:"draft"`
	Assets []releaseAsset `json:"assets"`
}

func (r *Resolver) get(ctx context.Context, location string) ([]byte, int, error) {
	if !HTTPS(location) {
		return nil, 0, errors.New("packslip downloads must use HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "bine")
	if req.URL.Host == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
		if r.Token != "" {
			req.Header.Set("Authorization", "Bearer "+r.Token)
		}
	}
	// Never forward credentials to signed artifact hosts or allow HTTPS to HTTP
	// redirects. Preserve the caller's transport (including test transports).
	client := *r.Client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 10 {
			return errors.New("unsafe or excessive Packslip redirect")
		}
		if req.URL.Host != "api.github.com" {
			req.Header.Del("Authorization")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("packslip request %s: %s", location, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(data) > maxMetadata {
		return nil, resp.StatusCode, errors.New("packslip metadata exceeds size limit")
	}
	return data, resp.StatusCode, nil
}

func (r *Resolver) discover(ctx context.Context, request Request) (repository, []githubRelease, error) {
	var repo repository
	name, tool, err := Project(request.Project)
	if err != nil {
		return repo, nil, err
	}
	data, status, err := r.get(ctx, "https://api.github.com/repos/"+name)
	if err != nil {
		return repo, nil, err
	}
	if status != http.StatusOK {
		return repo, nil, errors.New("packslip repository not found")
	}
	if err := json.Unmarshal(data, &repo); err != nil {
		return repo, nil, err
	}
	if _, sub, err := Project("github.com/" + repo.FullName); err != nil || sub != "" || repo.ID <= 0 || repo.Owner.ID <= 0 || repo.DefaultBranch == "" {
		return repo, nil, errors.New("invalid GitHub repository metadata")
	}
	repo.aliases = []string{name}
	pinned := false
	trust, err := r.readTrust()
	if err != nil {
		return repo, nil, err
	}
	for project, p := range trust.Projects {
		if project != request.Project && (p.RepositoryID != strconv.FormatInt(repo.ID, 10) || p.Tool != tool) {
			continue
		}
		if p.RepositoryID != strconv.FormatInt(repo.ID, 10) || p.OwnerID != strconv.FormatInt(repo.Owner.ID, 10) {
			return repo, nil, errors.New("GitHub repository or owner differs from remembered Packslip trust")
		}
		pinned = true
		// Tags can retain repository names from before a rename.
		if name, _, err := Project(project); err == nil {
			repo.aliases = append(repo.aliases, name)
		}
		if name, _, err := Project(strings.TrimPrefix(p.Repository, "https://")); err == nil {
			repo.aliases = append(repo.aliases, name)
		}
	}
	if !pinned && !strings.EqualFold(strings.Split(name, "/")[0], strings.Split(repo.FullName, "/")[0]) {
		return repo, nil, errors.New("GitHub redirected to another owner; configure the intended project explicitly after reviewing the transfer")
	}
	list := ".well-known/packslip.json"
	if tool != "" {
		list = ".well-known/packslip/" + tool + ".json"
	}
	_, status, err = r.get(ctx, "https://api.github.com/repos/"+repo.FullName+"/contents/"+list+"?ref="+url.QueryEscape(repo.DefaultBranch))
	if err != nil {
		return repo, nil, err
	}
	if status != http.StatusNotFound {
		return repo, nil, errors.New("supplementary signed Packslip release lists are not supported yet; refusing to ignore release policy")
	}
	var releases []githubRelease
	// Bound discovery work and fail rather than silently treat a partial list as
	// complete. GitHub's order and prerelease flag do not define version order.
	for page := 1; page <= 100; page++ {
		data, status, err := r.get(ctx, fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", repo.FullName, page))
		if err != nil {
			return repo, nil, err
		}
		if status != http.StatusOK {
			return repo, nil, errors.New("packslip release discovery failed")
		}
		var batch []githubRelease
		if err := json.Unmarshal(data, &batch); err != nil {
			return repo, nil, err
		}
		releases = append(releases, batch...)
		if len(batch) < 100 {
			return repo, releases, nil
		}
	}
	return repo, nil, errors.New("packslip release discovery exceeded 100 pages")
}

// tagVersion implements the conventional GitHub tag prefixes used for
// discovery. The signed manifest must still match this version exactly.
func tagVersion(tag, repo, tool string, aliases ...string) string {
	if v := normalizeTagVersion(tag); v != "" {
		return v
	}
	prefixes := []string{path.Base(repo)}
	for _, alias := range aliases {
		prefixes = append(prefixes, path.Base(alias))
	}
	if tool != "" {
		prefixes = append(prefixes, tool, path.Base(tool))
	}
	for _, prefix := range prefixes {
		for _, separator := range []string{"/", "-", "_", "@"} {
			if rest, ok := strings.CutPrefix(tag, prefix+separator); ok {
				if v := normalizeTagVersion(rest); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

func normalizeTagVersion(tag string) string {
	v := strings.TrimPrefix(tag, "v")
	if ValidVersion(v) {
		return v
	}
	// Vendor spellings may omit a patch or use leading zeroes/date separators.
	parts := strings.Split(v, ".")
	if len(parts) == 1 {
		parts = strings.Split(v, "-")
	}
	if len(parts) != 2 && len(parts) != 3 {
		return ""
	}
	for i, p := range parts {
		if p == "" || strings.IndexFunc(p, func(c rune) bool { return c < '0' || c > '9' }) >= 0 {
			return ""
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return ""
		}
		parts[i] = strconv.FormatUint(n, 10)
	}
	if len(parts) == 2 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".")
}

var errNoManifest = errors.New("release has no Packslip for the requested project")

func (r *Resolver) Resolve(ctx context.Context, request Request) (*Resolved, error) {
	if !ValidVersion(request.Version) {
		return nil, errors.New("packslip requires an exact semantic version")
	}
	repo, releases, err := r.discover(ctx, request)
	if err != nil {
		return nil, err
	}
	_, tool, _ := Project(request.Project)
	var found *Resolved
	for _, release := range releases {
		if release.Draft || tagVersion(release.Tag, repo.FullName, tool, repo.aliases...) != request.Version {
			continue
		}
		result, err := r.resolveRelease(ctx, request, repo, release)
		if errors.Is(err, errNoManifest) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if found != nil {
			return nil, errors.New("multiple releases declare the requested Packslip version")
		}
		found = result
	}
	if found == nil {
		return nil, fmt.Errorf("packslip %s@%s not found", request.Project, request.Version)
	}
	return found, nil
}

// Latest returns the highest stable version having a verified, compatible
// Packslip. This is an upgrade check, not an unconstrained 'latest' install.
func (r *Resolver) Latest(ctx context.Context, request Request) (string, error) {
	repo, releases, err := r.discover(ctx, request)
	if err != nil {
		return "", err
	}
	_, tool, _ := Project(request.Project)
	slices.SortFunc(releases, func(a, b githubRelease) int {
		return -semver.Compare("v"+tagVersion(a.Tag, repo.FullName, tool, repo.aliases...), "v"+tagVersion(b.Tag, repo.FullName, tool, repo.aliases...))
	})
	for _, release := range releases {
		version := tagVersion(release.Tag, repo.FullName, tool, repo.aliases...)
		if release.Draft || version == "" || semver.Prerelease("v"+version) != "" {
			continue
		}
		request.Version = version
		_, err := r.resolveRelease(ctx, request, repo, release)
		if errors.Is(err, errNoManifest) || errors.Is(err, ErrNoArtifact) {
			continue
		}
		if err != nil {
			return "", err
		}
		return version, nil
	}
	return "", errors.New("no stable Packslip release found")
}

func (r *Resolver) resolveRelease(ctx context.Context, request Request, repo repository, release githubRelease) (*Resolved, error) {
	_, tool, _ := Project(request.Project)
	var found *Resolved
	for _, asset := range release.Assets {
		if !strings.HasPrefix(asset.Name, "packslip") || !strings.HasSuffix(asset.Name, ".sigstore.json") {
			continue
		}
		data, status, err := r.get(ctx, asset.URL)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, errors.New("advertised Packslip bundle is missing")
		}
		// Use the untrusted project only to skip bundles for other monorepo tools.
		// It never establishes the identity accepted below.
		var envelope struct {
			DSSE struct {
				Payload string `json:"payload"`
			} `json:"dsseEnvelope"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, err
		}
		payload, err := base64.StdEncoding.DecodeString(envelope.DSSE.Payload)
		if err != nil {
			return nil, err
		}
		var hint struct {
			Predicate struct {
				Project string `json:"project"`
			} `json:"predicate"`
		}
		if err := json.Unmarshal(payload, &hint); err != nil {
			return nil, err
		}
		_, hintedTool, err := Project(hint.Predicate.Project)
		if err != nil || hintedTool != tool {
			continue
		}
		verify := r.VerifyBundle
		if verify == nil {
			verify = func(ctx context.Context, data []byte) ([]byte, Identity, error) {
				root, err := trustedRoot(ctx, filepath.Join(r.StateDir, "sigstore"))
				if err != nil {
					return nil, Identity{}, err
				}
				return VerifyBundle(data, root)
			}
		}
		payload, identity, err := verify(ctx, data)
		if err != nil {
			return nil, err
		}
		s, err := Parse(payload)
		if err != nil {
			return nil, fmt.Errorf("invalid Packslip: %w", err)
		}
		p := s.Predicate
		signedRepo, signedTool, _ := Project(p.Project)
		if signedTool != tool || identity.RepositoryID != strconv.FormatInt(repo.ID, 10) || identity.OwnerID != strconv.FormatInt(repo.Owner.ID, 10) || identity.Repository != "https://github.com/"+signedRepo || !strings.HasPrefix(identity.Signer, identity.Repository+"/.github/workflows/") || identity.Signer != p.Identity.KeyID {
			return nil, errors.New("packslip signer or project does not match the requested GitHub repository and owner")
		}
		if p.Version != request.Version || (p.Source != nil && p.Source.Tag != "" && p.Source.Tag != release.Tag) {
			return nil, errors.New("packslip version or source tag does not match the requested release")
		}
		a, command, err := s.Select(r.Host, request.Variant, request.Command, request.Name)
		if err != nil {
			return nil, err
		}
		if a.URL == "" {
			for _, asset := range release.Assets {
				if asset.Name == a.Name {
					a.URL = asset.URL
					break
				}
			}
			if !HTTPS(a.URL) {
				return nil, errors.New("selected artifact has no HTTPS download URL")
			}
		}
		if found != nil {
			return nil, errors.New("multiple Packslip manifests match the requested project")
		}
		sum := sha256.Sum256(data)
		found = &Resolved{p.Project, p.Version, hex.EncodeToString(sum[:]), a, command, s.Digests(a.Name), identity}
		trust, err := r.readTrust()
		if err != nil {
			return nil, err
		}
		if err := trust.check(request.Project, identity, a, r.Host); err != nil {
			return nil, err
		}
	}
	if found == nil {
		return nil, errNoManifest
	}
	return found, nil
}

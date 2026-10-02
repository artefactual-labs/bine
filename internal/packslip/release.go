package packslip

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const ReleaseType = "https://packslip.dev/release/v1"

type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Release   `json:"predicate"`
}

type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type Release struct {
	Project     string     `json:"project"`
	Version     string     `json:"version"`
	PublishedAt string     `json:"published_at"`
	Artifacts   []Artifact `json:"artifacts"`
	AttestedBy  string     `json:"attested_by"`
	Identity    struct {
		Scheme string `json:"scheme"`
		KeyID  string `json:"key_id"`
		Issuer string `json:"issuer"`
	} `json:"identity"`
	Source *struct {
		Repo   string `json:"repo"`
		Commit string `json:"commit"`
		Tag    string `json:"tag"`
	} `json:"source"`
	Resources []struct {
		Kind    string   `json:"kind"`
		Asset   string   `json:"asset"`
		Archive string   `json:"archive"`
		Repo    string   `json:"repo"`
		Exec    []string `json:"exec"`
	} `json:"resources"`
}

type Artifact struct {
	Name       string       `json:"name"`
	OS         string       `json:"os"`
	Arch       string       `json:"arch"`
	Libc       string       `json:"libc"`
	Variant    string       `json:"variant"`
	Size       *int64       `json:"size"`
	URL        string       `json:"url"`
	Format     string       `json:"format"`
	Bin        []Executable `json:"bin"`
	Requires   Requirements `json:"requires"`
	Provenance []string     `json:"provenance"`
}

type Requirements struct {
	OSMin    string   `json:"os_min"`
	GlibcMin string   `json:"glibc_min"`
	Libs     []string `json:"libs"`
	Bin      []struct {
		Name string `json:"name"`
		Min  string `json:"min"`
	} `json:"bin"`
}

type Executable struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (e *Executable) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &e.Path); err != nil {
			return err
		}
		e.Name = strings.TrimSuffix(path.Base(e.Path), ".exe")
		return nil
	}
	type plain Executable
	return json.Unmarshal(data, (*plain)(e))
}

var (
	wordPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)
	projectPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// Project separates a GitHub trust subject into its repository and tool path.
func Project(name string) (repo, tool string, err error) {
	parts := strings.Split(name, "/")
	if len(parts) < 3 || parts[0] != "github.com" {
		return "", "", errors.New("packslip must name github.com/owner/repo[/tool]")
	}
	for _, p := range parts[1:] {
		if p == "." || p == ".." || !projectPart.MatchString(p) {
			return "", "", errors.New("invalid Packslip project path")
		}
	}
	return strings.Join(parts[1:3], "/"), strings.Join(parts[3:], "/"), nil
}

func ValidVersion(v string) bool {
	return !strings.HasPrefix(v, "v") && semver.IsValid("v"+v) && strings.Count(strings.SplitN(strings.SplitN(v, "+", 2)[0], "-", 2)[0], ".") == 2
}

func SafePath(p string) bool {
	return p != "." && fs.ValidPath(p) && !strings.ContainsAny(p, "\\:") && strings.IndexFunc(p, func(r rune) bool { return r < 32 || r == 127 }) < 0
}

func CommandName(s string) bool { return SafePath(s) && !strings.Contains(s, "/") }

func HTTPS(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}

func Parse(data []byte) (*Statement, error) {
	// Reject duplicate keys, including unknown fields: parsers must not disagree
	// on the meaning of a signed document.
	d := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSON(d); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON in Packslip statement")
	}
	var s Statement
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func uniqueJSON(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			k, ok := key.(string)
			if !ok || keys[k] {
				return fmt.Errorf("duplicate or invalid JSON key %v", key)
			}
			keys[k] = true
		}
		if err := uniqueJSON(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func (s *Statement) validate() error {
	r := &s.Predicate
	if s.Type != "https://in-toto.io/Statement/v1" || s.PredicateType != ReleaseType {
		return errors.New("expected a Packslip release/v1 statement")
	}
	if _, _, err := Project(r.Project); err != nil {
		return err
	}
	if !ValidVersion(r.Version) {
		return fmt.Errorf("invalid Packslip semantic version %q", r.Version)
	}
	if _, err := time.Parse(time.RFC3339Nano, r.PublishedAt); err != nil || !strings.HasSuffix(r.PublishedAt, "Z") {
		return errors.New("invalid Packslip publication time")
	}
	if r.Identity.Scheme != "sigstore-oidc" || r.Identity.Issuer != githubIssuer || r.Identity.KeyID == "" || (r.AttestedBy != "" && r.AttestedBy != "vendor") {
		return errors.New("only vendor manifests signed by GitHub Actions are supported")
	}
	if r.Source != nil && !HTTPS(r.Source.Repo) {
		return errors.New("invalid Packslip source repository")
	}
	subjects := map[string]bool{}
	for _, sub := range s.Subject {
		if !CommandName(sub.Name) || subjects[sub.Name] || !validDigest(sub.Digest["sha256"], 32) {
			return errors.New("invalid or duplicate Packslip subject")
		}
		if v, ok := sub.Digest["sha512"]; ok && !validDigest(v, 64) {
			return errors.New("invalid SHA-512 digest")
		}
		subjects[sub.Name] = true
	}
	used, platforms := map[string]bool{}, map[string]bool{}
	if len(r.Artifacts) == 0 {
		return errors.New("packslip release has no artifacts")
	}
	for _, a := range r.Artifacts {
		if !subjects[a.Name] || used[a.Name] || a.Size == nil || *a.Size < 0 || a.Format == "" {
			return fmt.Errorf("invalid or duplicate artifact %q", a.Name)
		}
		used[a.Name] = true
		for _, w := range []string{a.OS, a.Arch, a.Libc, a.Variant, a.Format} {
			if w != "" && !wordPattern.MatchString(w) {
				return fmt.Errorf("invalid artifact vocabulary %q", w)
			}
		}
		if a.URL != "" && !HTTPS(a.URL) {
			return errors.New("artifact URLs must use HTTPS")
		}
		key := strings.Join([]string{a.OS, a.Arch, a.Libc, a.Variant, a.Format}, "/")
		if platforms[key] {
			return errors.New("duplicate artifact platform, variant and format")
		}
		platforms[key] = true
		names := map[string]bool{}
		for _, e := range a.Bin {
			if !SafePath(e.Path) || !CommandName(e.Name) || names[e.Name] {
				return errors.New("invalid or duplicate executable entry")
			}
			names[e.Name] = true
			if a.Format == "raw" && e.Path != a.Name {
				return errors.New("raw executable path must match the artifact name")
			}
		}
		for _, p := range a.Provenance {
			if !HTTPS(p) {
				return errors.New("invalid provenance URL")
			}
		}
		for _, cmd := range a.Requires.Bin {
			if !CommandName(cmd.Name) {
				return errors.New("invalid required command")
			}
		}
	}
	for _, resource := range r.Resources {
		count := 0
		for _, source := range []string{resource.Asset, resource.Archive, resource.Repo} {
			if source != "" {
				count++
				if !SafePath(source) {
					return errors.New("invalid resource path")
				}
			}
		}
		if len(resource.Exec) > 0 {
			count++
		}
		if count != 1 || resource.Kind == "" {
			return errors.New("invalid resource source")
		}
		if resource.Asset != "" {
			if !subjects[resource.Asset] {
				return errors.New("resource asset is missing its subject")
			}
			used[resource.Asset] = true
		}
		if resource.Repo != "" && (r.Source == nil || r.Source.Commit == "") {
			return errors.New("repository resource requires a source commit")
		}
	}
	if len(used) != len(subjects) {
		return errors.New("subject is neither an artifact nor a resource asset")
	}
	return nil
}

func validDigest(s string, n int) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == n && strings.ToLower(s) == s
}

type Host struct{ OS, Arch, Libc string }

// MatchesHost reports whether the artifact's platform fields fit the host.
func (a Artifact) MatchesHost(host Host) bool {
	return (a.OS == "" || a.OS == host.OS) && (a.Arch == "" || a.Arch == host.Arch) && (a.Libc == "" || a.Libc == host.Libc)
}

// Format preference breaks otherwise equal platform matches. Installers and
// single-file compression formats are intentionally outside the initial scope.
var formats = []string{"tar.xz", "tar.zst", "tar.gz", "tgz", "tar.bz2", "tar", "zip", "raw"}

var ErrNoArtifact = errors.New("no compatible Packslip artifact")

func (s *Statement) Select(host Host, variant, command, localName string) (Artifact, Executable, error) {
	var candidates []Artifact
	best, rank := -1, len(formats)
	for _, a := range s.Predicate.Artifacts {
		format := slices.Index(formats, a.Format)
		if format < 0 || a.Variant != variant || !a.MatchesHost(host) {
			continue
		}
		specificity := 0
		for _, v := range []string{a.OS, a.Arch, a.Libc} {
			if v != "" {
				specificity++
			}
		}
		if specificity > best || (specificity == best && format < rank) {
			candidates, best, rank = nil, specificity, format
		}
		if specificity == best && format == rank {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return Artifact{}, Executable{}, fmt.Errorf("%w: found 0 for %s/%s/%s variant %q", ErrNoArtifact, host.OS, host.Arch, host.Libc, variant)
	}
	if len(candidates) != 1 {
		return Artifact{}, Executable{}, fmt.Errorf("expected one compatible artifact, found %d for %s/%s/%s variant %q", len(candidates), host.OS, host.Arch, host.Libc, variant)
	}
	a := candidates[0]
	for _, e := range a.Bin {
		if (command != "" && e.Name == command) || (command == "" && e.Name == localName) {
			return a, e, nil
		}
	}
	if command == "" && len(a.Bin) == 1 {
		return a, a.Bin[0], nil
	}
	names := make([]string, len(a.Bin))
	for i, e := range a.Bin {
		names[i] = e.Name
	}
	return Artifact{}, Executable{}, fmt.Errorf("select command explicitly; available commands: %s", strings.Join(names, ", "))
}

func (s *Statement) Digests(name string) map[string]string {
	for _, sub := range s.Subject {
		if sub.Name == name {
			return sub.Digest
		}
	}
	return nil
}

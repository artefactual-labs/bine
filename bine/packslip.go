package bine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/artefactual-labs/bine/internal/packslip"
)

type packslipConfig struct {
	Project string `json:"project" toml:"project"`
	Command string `json:"command,omitempty" toml:"command,omitempty"`
	Variant string `json:"variant,omitempty" toml:"variant,omitempty"`
}

// Reject misspelled selectors without tightening legacy configuration parsing.
func (p *packslipConfig) UnmarshalJSON(data []byte) error {
	type plain packslipConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*plain)(p)); err != nil {
		return fmt.Errorf("decode packslip configuration: %w", err)
	}
	return nil
}

type packslipReceipt struct {
	Request          packslip.Request  `json:"request"`
	Release          packslip.Resolved `json:"release"`
	ExecutableSHA256 string            `json:"executable_sha256"`
}

func (b *bin) validatePackslip() error {
	if b.Packslip == nil {
		return nil
	}
	if b.URL != "" || b.GoPackage != "" || b.AssetPattern != "" || b.TagPattern != "" || len(b.Modifiers) != 0 {
		return errors.New("packslip cannot be combined with url, go_package, asset_pattern, tag_pattern or modifiers")
	}
	if b.Packslip.Project == "" {
		return errors.New("packslip.project is required")
	}
	if _, _, err := packslip.Project(b.Packslip.Project); err != nil {
		return err
	}
	if !packslip.ValidVersion(strings.TrimPrefix(b.Version, "v")) {
		return errors.New("packslip requires an exact version, such as 1.2.3")
	}
	if !packslip.CommandName(b.Name) || (b.Packslip.Command != "" && !packslip.CommandName(b.Packslip.Command)) {
		return errors.New("invalid Packslip command name")
	}
	if b.Packslip.Variant != "" && !regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`).MatchString(b.Packslip.Variant) {
		return errors.New("invalid Packslip variant")
	}
	return nil
}

func (b *bin) packslipRequest() packslip.Request {
	return packslip.Request{Project: b.Packslip.Project, Version: strings.TrimPrefix(b.Version, "v"), Command: b.Packslip.Command, Name: b.Name, Variant: b.Packslip.Variant}
}

type packslipSource struct {
	client   *http.Client
	token    string
	stateDir string
	logger   logr.Logger
	// Uses the resolver's existing verification seam for offline lifecycle tests.
	verifyBundle func(context.Context, []byte) ([]byte, packslip.Identity, error)
}

func (p *packslipSource) resolver(ctx context.Context) (*packslip.Resolver, error) {
	dir := p.stateDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, "bine", "packslip")
	}
	return &packslip.Resolver{Client: p.client, Token: p.token, StateDir: dir, Host: packslipHost(ctx), VerifyBundle: p.verifyBundle}, nil
}

func (p *packslipSource) latestVersion(ctx context.Context, b *bin) (string, error) {
	r, err := p.resolver(ctx)
	if err != nil {
		return "", err
	}
	return r.Latest(ctx, b.packslipRequest())
}

func (s *packslipSource) validateMarker(ctx context.Context, b *bin, marker *versionMarkerDocument) (bool, error) {
	receipt := marker.Packslip
	if receipt == nil || receipt.Request != b.packslipRequest() || receipt.ExecutableSHA256 == "" || !marker.Checksum.Matches(receipt.ExecutableSHA256) {
		return false, nil
	}
	resolver, err := s.resolver(ctx)
	if err != nil {
		return false, err
	}
	if !receipt.Release.Artifact.MatchesHost(resolver.Host) {
		return false, nil
	}
	return resolver.Accepted(b.Packslip.Project, &receipt.Release)
}

func packslipHost(ctx context.Context) packslip.Host {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	arch := goarch
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	case "386":
		arch = "i686"
	case "ppc64le":
		arch = "powerpc64le"
	case "loong64":
		arch = "loongarch64"
	case "arm":
		// Do not infer the ARM generation from the Go binary's build target.
		if data, err := execCommand(ctx, "uname", "-m").Output(); err == nil {
			switch strings.TrimSpace(string(data)) {
			case "armv7l":
				arch = "armv7"
			case "armv6l":
				arch = "armv6"
			}
		}
	}
	host := packslip.Host{OS: goos, Arch: arch}
	if goos == "linux" {
		// Only advertise a known libc. An unknown host may still use static builds.
		if data, _ := execCommand(ctx, "ldd", "--version").CombinedOutput(); strings.Contains(strings.ToLower(string(data)), "musl") {
			host.Libc = "musl"
		} else if strings.Contains(strings.ToLower(string(data)), "glibc") || strings.Contains(string(data), "GNU libc") || strings.Contains(string(data), "GNU C Library") {
			host.Libc = "gnu"
		}
		if host.Libc == "" {
			if _, err := os.Stat("/etc/alpine-release"); err == nil {
				host.Libc = "musl"
			}
		}
	}
	return host
}

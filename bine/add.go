package bine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// AddOptions describes a new tool. Exactly one of GoPackage, PackslipProject,
// and URL is required. Empty Version or "latest" resolves an exact version.
type AddOptions struct {
	Name            string
	GoPackage       string
	PackslipProject string
	URL             string
	Version         string
	Command         string
	Variant         string
	AssetPattern    string
	TagPattern      string
	NoInstall       bool
}

// Add installs a new tool and appends its exact version to the configuration.
// NoInstall skips installation, but latest-version resolution still needs
// upstream access. Failures do not add an entry; installation and trust state
// may remain if saving the configuration fails.
func (b *Bine) Add(ctx context.Context, opts AddOptions) (*ListItem, error) {
	tool, err := opts.bin()
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	for _, existing := range b.config.Bins {
		if existing.Name == tool.Name {
			return nil, fmt.Errorf("add: binary %q already exists", tool.Name)
		}
	}
	tool.source, err = b.sources.newSource(ctx, tool)
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	if tool.Version == "" || strings.EqualFold(tool.Version, "latest") {
		if tool.goPkg() {
			tool.Version, err = goResolveVersion(ctx, tool.GoPackage)
		} else {
			tool.Version, err = tool.source.latestVersion(ctx, tool)
		}
		if err != nil {
			return nil, fmt.Errorf("add: resolve version for %q: %w", tool.Name, err)
		}
	}
	version := semver.Canonical("v" + strings.TrimPrefix(tool.Version, "v"))
	if version == "" {
		return nil, fmt.Errorf("add: invalid version %q", tool.Version)
	}
	if tool.Packslip == nil {
		tool.Version = strings.TrimPrefix(version, "v")
	} else {
		tool.Version = strings.TrimPrefix(tool.Version, "v")
	}
	if err := tool.validatePackslip(); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	edit, err := b.config.prepareAddition(tool)
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	if !opts.NoInstall {
		if _, err := b.installVersion(ctx, tool, ""); err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := edit.save(); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	b.config.Bins = append(b.config.Bins, tool)
	return &ListItem{Name: tool.Name, Version: tool.usableVersion()}, nil
}

func (opts AddOptions) bin() (*bin, error) {
	if err := validateLocalName(opts.Name); err != nil {
		return nil, fmt.Errorf("invalid binary name: %w", err)
	}
	count := 0
	for _, source := range []string{opts.GoPackage, opts.PackslipProject, opts.URL} {
		if source != "" {
			count++
		}
	}
	if count != 1 {
		return nil, errors.New("exactly one source is required: --go, --packslip or --url")
	}
	if opts.PackslipProject == "" && (opts.Command != "" || opts.Variant != "") {
		return nil, errors.New("--command and --variant require --packslip")
	}
	if opts.URL == "" && (opts.AssetPattern != "" || opts.TagPattern != "") {
		return nil, errors.New("--asset-pattern and --tag-pattern require --url")
	}
	tool := &bin{Name: opts.Name, Version: opts.Version, GoPackage: opts.GoPackage, URL: opts.URL, AssetPattern: opts.AssetPattern, TagPattern: opts.TagPattern}
	if opts.PackslipProject != "" {
		tool.Packslip = &packslipConfig{Project: opts.PackslipProject, Command: opts.Command, Variant: opts.Variant}
		// Validate selectors before discovery, which does not need a version.
		probe := *tool
		if probe.Version == "" || strings.EqualFold(probe.Version, "latest") {
			probe.Version = "0.0.0"
		}
		if err := probe.validatePackslip(); err != nil {
			return nil, err
		}
	}
	applyLibraryDefaults(&config{Bins: []*bin{tool}})
	if tool.URL != "" && tool.AssetPattern == "" {
		return nil, errors.New("--asset-pattern is required for a URL without a built-in recipe")
	}
	if tool.Version != "" && !strings.EqualFold(tool.Version, "latest") && semver.Canonical("v"+strings.TrimPrefix(tool.Version, "v")) == "" {
		return nil, fmt.Errorf("invalid version %q; use a release version or latest", tool.Version)
	}
	return tool, nil
}

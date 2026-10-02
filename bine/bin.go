package bine

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

type bin struct {
	Name    string `json:"name" toml:"name"`
	Version string `json:"version" toml:"version"`

	// Fields for asset-based downloads.
	URL          string `json:"url,omitempty" toml:"url,omitempty"`
	AssetPattern string `json:"asset_pattern,omitempty" toml:"asset_pattern,omitempty"`

	// Template for tag formatting. Supports {version} placeholder.
	// Defaults to "v{version}" if not specified.
	TagPattern string `json:"tag_pattern,omitempty" toml:"tag_pattern,omitempty"`

	// Field for go-based installs.
	GoPackage string `json:"go_package,omitempty" toml:"go_package,omitempty"`

	// Packslip selects a signed release and its executable.
	Packslip *packslipConfig `json:"packslip,omitempty" toml:"packslip,omitempty"`

	// Allows to apply modifications during variable expansion.
	Modifiers map[string]map[string]string `json:"modifiers,omitempty" toml:"modifiers,omitempty"`

	// source owns installation and upstream discovery, independent of version.
	source installationSource
}

func (b bin) goPkg() bool {
	return b.GoPackage != ""
}

// isLatest returns true if this binary tracks the latest available version.
// Only applicable to Go packages.
func (b bin) isLatest() bool {
	if !b.goPkg() {
		return false
	}
	return b.Version == "" || strings.EqualFold(b.Version, "latest")
}

// markerVersion returns the version string used for the version marker file.
// For "latest" bins, always returns "latest" regardless of whether the version
// field is empty or explicitly set to "latest".
func (b bin) markerVersion() string {
	if b.isLatest() {
		return "latest"
	}
	return b.Version
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
	if b.Packslip != nil {
		return "v" + strings.TrimPrefix(b.Version, "v")
	}
	version := b.canonicalVersion()
	if version == "" {
		return b.Version
	}
	return version
}

func (b bin) tagPattern() string {
	if b.TagPattern == "" {
		return "v{version}"
	}
	return b.TagPattern
}

// tag returns the tag name based on the tag template and version.
// If no tag template is specified, defaults to "v{version}".
func (b bin) tag() string {
	template := b.tagPattern()
	template = strings.ReplaceAll(template, "{version}", b.unprefixedVersion())
	template = strings.ReplaceAll(template, "{name}", b.Name)
	return template
}

// checkOutdated checks if the binary is outdated by comparing its version with
// the latest version available. For "latest" bins, resolvedVersion must be
// provided (the actual version currently installed, obtained from the version
// marker) since "latest" itself is not a comparable semver.
func (b *bin) checkOutdated(ctx context.Context, resolvedVersion string) (bool, string, error) {
	// Determine the version to compare against the latest.
	currentVersion := resolvedVersion
	if currentVersion == "" {
		currentVersion = b.Version
	}
	if currentVersion == "" || strings.EqualFold(currentVersion, "latest") {
		// This happens for "latest" Go bins when no resolved version has been
		// stored yet (e.g., the binary was installed by an older bine version),
		// or when a non-Go bin has an empty version field.
		return false, "", fmt.Errorf("binary %q has no resolved version to compare", b.Name)
	}

	latestVersion, err := b.source.latestVersion(ctx, b)
	if err != nil {
		return false, "", fmt.Errorf("check failed for binary %q: %v", b.Name, err)
	}

	// Compare versions using semver.
	current := semver.Canonical("v" + strings.TrimPrefix(currentVersion, "v"))
	latest := semver.Canonical("v" + strings.TrimPrefix(latestVersion, "v"))
	if latest == "" {
		return false, "", fmt.Errorf("invalid semver for latest version %q of %s", latest, b.Name)
	}

	isOutdated := semver.Compare(current, latest) < 0

	return isOutdated, latestVersion, nil
}

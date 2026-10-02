package bine

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
)

// An installation source retains strategy and dependencies, not bin versions.
// Bin configuration and requests are read-only inputs to its operations.
type installationSource interface {
	// install publishes the executable at target and returns marker metadata.
	// The caller writes the marker; errors may leave partial progress.
	install(ctx context.Context, request installRequest, target string) (installResult, error)

	latestVersion(ctx context.Context, b *bin) (string, error)

	// validateMarker checks source-specific requirements for reusing a marker.
	// True permits reuse subject to the caller's checksum check. False with nil
	// error requests a reinstall; an error stops the operation.
	validateMarker(ctx context.Context, b *bin, marker *versionMarkerDocument) (bool, error)
}

// An install request can override the installed version without changing the
// configured bin or its marker path.
type installRequest struct {
	Bin             *bin
	VersionOverride string
}

func (r installRequest) effectiveBin() *bin {
	if r.VersionOverride == "" {
		return r.Bin
	}
	b := *r.Bin
	b.Version = r.VersionOverride
	return &b
}

type installResult struct {
	ResolvedVersion string
}

// The factory shares host naming information between recipes. Source creation
// does not resolve upstream versions.
type sourceFactory struct {
	client *http.Client
	token  string
	logger logr.Logger
	namer  *namer
}

func (f *sourceFactory) newSource(ctx context.Context, b *bin) (installationSource, error) {
	switch {
	case b.goPkg():
		return &goSource{client: f.client, logger: f.logger}, nil
	}

	var provider recipeProvider
	switch {
	case strings.Contains(b.URL, "github.com"):
		provider = &githubProvider{client: f.client, token: f.token}
	case strings.Contains(b.URL, "release.ariga.io"):
		provider = &arigaProvider{client: f.client, token: f.token}
	default:
		return nil, fmt.Errorf("unsupported binary provider for %q (%s)", b.Name, b.URL)
	}
	if f.namer == nil {
		n, err := createNamer(ctx)
		if err != nil {
			return nil, fmt.Errorf("load config namer: %w", err)
		}
		f.namer = n
	}
	return &recipeSource{client: f.client, provider: provider, namer: f.namer}, nil
}

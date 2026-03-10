package bine

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/renameio/v2"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/mod/semver"
)

const (
	cacheDirName = "bine"
	userAgent    = "bine"
)

type Bine struct {
	logger logr.Logger
	client *http.Client
	config *config

	Project     string // Project name.
	CacheDir    string // e.g. ~/.cache/bine/project/linux/amd64/
	BinDir      string // e.g. ~/.cache/bine/project/linux/amd64/bin/
	VersionsDir string // e.g. ~/.cache/bine/project/linux/amd64/versions/
}

// New creates a new Bine instance with default options.
func New() (*Bine, error) {
	return newBine(context.Background(), nil)
}

// NewWithOptions creates a new Bine instance with the given options.
func NewWithOptions(opts ...Option) (*Bine, error) {
	optsConfig := options{}
	for _, opt := range opts {
		if err := opt(&optsConfig); err != nil {
			return nil, err
		}
	}

	ctx := optsConfig.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return newBine(ctx, &optsConfig)
}

// Option configures a Bine instance (used by NewWithOptions).
type Option func(*options) error

type options struct {
	ctx          context.Context
	logger       *logr.Logger
	cacheDirBase string
	ghAPIToken   string
}

// WithContext specifies a custom context for the Bine instance.
// This context is only used during the initial configuration load.
func WithContext(ctx context.Context) Option {
	return func(o *options) error {
		o.ctx = ctx
		return nil
	}
}

// WithLogger specifies a custom logger for the Bine instance.
func WithLogger(logger logr.Logger) Option {
	return func(o *options) error {
		o.logger = &logger
		return nil
	}
}

// WithCacheDir specifies a custom base directory for the bine cache.
func WithCacheDir(path string) Option {
	return func(o *options) error {
		o.cacheDirBase = path
		return nil
	}
}

// WithGitHubAPIToken specifies a GitHub API token for authentication.
func WithGitHubAPIToken(token string) Option {
	return func(o *options) error {
		o.ghAPIToken = token
		return nil
	}
}

// newBine creates a new Bine instance with the given options.
func newBine(ctx context.Context, optsConfig *options) (*Bine, error) {
	if optsConfig == nil {
		optsConfig = &options{}
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 3
	stdClient := client.StandardClient()

	config, err := loadConfig(ctx, stdClient, optsConfig.ghAPIToken)
	if err != nil {
		return nil, err
	}

	b := &Bine{
		client:  stdClient,
		config:  config,
		Project: config.Project,
	}

	if optsConfig.logger != nil {
		b.logger = *optsConfig.logger
		client.Logger = clientLogger{b.logger.WithName("client")}
	}

	if cacheDir, err := b.cacheDir(optsConfig.cacheDirBase); err != nil {
		return nil, err
	} else {
		b.CacheDir = cacheDir
		b.BinDir = filepath.Join(cacheDir, "bin")
		b.VersionsDir = filepath.Join(cacheDir, "versions")
	}

	return b, nil
}

// cacheDir returns the cache directory for the given project.
//
// Only called once at startup.
func (b *Bine) cacheDir(baseDir string) (string, error) {
	project := b.config.Project

	var err error
	if baseDir == "" {
		baseDir, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(baseDir, cacheDirName)
	}

	cacheDir := filepath.Join(baseDir, project, runtime.GOOS, runtime.GOARCH)

	b.logger.V(1).Info("Cache directory identified.", "path", cacheDir)

	return cacheDir, nil
}

// load the config of a binary given its name.
func (b *Bine) load(name string) (*bin, error) {
	var bin *bin
	for _, item := range b.config.Bins {
		if item.Name == name {
			bin = item
		}
	}

	if bin == nil {
		return nil, fmt.Errorf("binary %q not found", name)
	}

	return bin, nil
}

// install ensures that the given binary is installed.
func (b *Bine) install(ctx context.Context, bin *bin) (_ string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%q: %v", bin.Name, err)
		}
	}()

	binPath := filepath.Join(b.BinDir, bin.Name)

	// If version marker exists, assume binary is already installed.
	if ok, err := b.installed(ctx, bin); ok {
		return binPath, nil
	} else if err != nil {
		return "", fmt.Errorf("failed to check if binary is installed: %v", err)
	}

	// Ensure the bin directory exists.
	if err := os.MkdirAll(b.BinDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %v", err)
	}

	var resolvedVersion string
	if bin.goPkg() {
		if err := goInstall(ctx, bin, b.BinDir); err != nil {
			return "", fmt.Errorf("failed to install Go tool: %v", err)
		}
		// For "latest" bins, resolve the actual installed version so we can
		// detect upgrades in the future.
		if bin.isLatest() {
			if v, err := goInstalledVersion(ctx, binPath); err != nil {
				b.logger.V(1).Info("Could not determine installed version for 'latest' tracking.", "bin", bin.Name, "err", err)
			} else {
				resolvedVersion = v
			}
		}
	} else {
		if err := binInstall(ctx, b.client, bin, binPath); err != nil {
			return "", fmt.Errorf("failed to install binary: %v", err)
		}
	}

	if err := b.markVersion(bin, resolvedVersion); err != nil {
		return "", err
	}

	return binPath, nil
}

// installed determines if a binary is already installed.
func (b *Bine) installed(ctx context.Context, bin *bin) (bool, error) {
	if bin.isLatest() {
		_, ok, err := b.latestResolvedVersion(ctx, bin)
		if err != nil {
			var resolveErr latestVersionResolutionError
			if errors.As(err, &resolveErr) {
				b.logger.V(1).Info("Could not validate latest-tracking installation; will reinstall.", "bin", bin.Name, "err", err)
				return false, nil
			}
			return false, err
		}
		return ok, nil
	}

	binPath := filepath.Join(b.BinDir, bin.Name)
	if info, err := os.Stat(binPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else if info.IsDir() {
		return false, fmt.Errorf("expected %q to be a file, but it's a directory", binPath)
	}

	versionMarker := filepath.Join(b.VersionsDir, bin.Name, bin.markerVersion())
	blob, err := os.ReadFile(versionMarker)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// The version marker file may be empty (if made by older versions of bine).
	// We report that the binary is not installed to ensure it's reinstalled
	// with the correct version marker.
	if len(blob) == 0 {
		return false, nil
	}

	var marker versionMarkerDocument
	if err := json.Unmarshal(blob, &marker); err != nil {
		return false, err
	}

	if sum, err := checksum(binPath); err != nil {
		return false, fmt.Errorf("checksum: %v", err)
	} else if !marker.Checksum.Matches(sum) {
		return false, nil
	}

	return true, nil
}

type versionMarkerChecksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

func (c versionMarkerChecksum) Matches(sum string) bool {
	return c.Algorithm == crypto.SHA256.String() && c.Value == sum
}

type versionMarkerDocument struct {
	Checksum versionMarkerChecksum `json:"checksum"`
	// ResolvedVersion is the actual version installed for "latest" bins.
	// It is empty for bins with a pinned version.
	ResolvedVersion string `json:"resolved_version,omitempty"`
}

type latestVersionResolutionError struct {
	err error
}

func (e latestVersionResolutionError) Error() string {
	return e.err.Error()
}

func (e latestVersionResolutionError) Unwrap() error {
	return e.err
}

// readVersionMarker reads the version marker file for the given binary.
func (b *Bine) readVersionMarker(bin *bin) (*versionMarkerDocument, error) {
	versionMarker := filepath.Join(b.VersionsDir, bin.Name, bin.markerVersion())
	blob, err := os.ReadFile(versionMarker)
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		return &versionMarkerDocument{}, nil
	}
	var marker versionMarkerDocument
	if err := json.Unmarshal(blob, &marker); err != nil {
		return nil, err
	}
	return &marker, nil
}

// latestResolvedVersion returns the installed version for a latest-tracking Go
// binary. It prefers the cached marker, but can recover the value from the
// binary itself and repair the marker if needed.
func (b *Bine) latestResolvedVersion(ctx context.Context, bin *bin) (string, bool, error) {
	binPath := filepath.Join(b.BinDir, bin.Name)
	info, err := os.Stat(binPath)
	if os.IsNotExist(err) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	} else if info.IsDir() {
		return "", false, fmt.Errorf("expected %q to be a file, but it's a directory", binPath)
	}

	sum, err := checksum(binPath)
	if err != nil {
		return "", true, fmt.Errorf("checksum: %v", err)
	}

	if marker, err := b.readVersionMarker(bin); err == nil {
		if marker.Checksum.Matches(sum) && marker.ResolvedVersion != "" {
			return marker.ResolvedVersion, true, nil
		}
	} else if !os.IsNotExist(err) {
		b.logger.V(1).Info("Could not read latest-tracking version marker; will recover from binary.", "bin", bin.Name, "err", err)
	}

	resolvedVersion, err := goInstalledVersion(ctx, binPath)
	if err != nil {
		return "", true, latestVersionResolutionError{err: fmt.Errorf("resolve installed version: %v", err)}
	}

	if err := b.markVersion(bin, resolvedVersion); err != nil {
		b.logger.V(1).Info("Could not repair latest-tracking version marker.", "bin", bin.Name, "err", err)
	}

	return resolvedVersion, true, nil
}

// markVersion creates a version marker file for the binary.
// resolvedVersion is the actual semver installed; it is only set for "latest"
// bins and is used to detect upgrades.
func (b *Bine) markVersion(bin *bin, resolvedVersion string) error {
	versionsDir := filepath.Join(b.VersionsDir, bin.Name)
	versionMarker := filepath.Join(versionsDir, bin.markerVersion())

	// Ensure the versions directory exists.
	if err := os.MkdirAll(versionsDir, 0o750); err != nil {
		return fmt.Errorf("mkdir versions dir: %v", err)
	}

	binPath := filepath.Join(b.BinDir, bin.Name)
	sum, err := checksum(binPath)
	if err != nil {
		return fmt.Errorf("checksum: %v", err)
	}

	doc := versionMarkerDocument{
		Checksum: versionMarkerChecksum{
			Algorithm: crypto.SHA256.String(),
			Value:     sum,
		},
		ResolvedVersion: resolvedVersion,
	}
	data, err := json.MarshalIndent(doc, "", "\t")
	if err != nil {
		return fmt.Errorf("json marshal: %v", err)
	}

	return renameio.WriteFile(versionMarker, data, 0o640, renameio.WithStaticPermissions(0o640))
}

// removeInstallation removes the cached binary and its version marker.
func (b *Bine) removeInstallation(bin *bin) error {
	versionMarker := filepath.Join(b.VersionsDir, bin.Name, bin.markerVersion())
	if err := os.Remove(versionMarker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove version marker: %v", err)
	}

	binPath := filepath.Join(b.BinDir, bin.Name)
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove binary: %v", err)
	}

	return nil
}

func (b *Bine) reinstall(ctx context.Context, bin *bin) error {
	if err := b.removeInstallation(bin); err != nil {
		return err
	}

	_, err := b.install(ctx, bin)
	return err
}

// Get retrieves the path to a binary given its name.
func (b *Bine) Get(ctx context.Context, name string) (string, error) {
	bin, err := b.load(name)
	if err != nil {
		return "", fmt.Errorf("get: %v", err)
	}

	path, err := b.install(ctx, bin)
	if err != nil {
		return "", fmt.Errorf("get: %v", err)
	}

	return path, nil
}

// Run runs a binary given its name and arguments.
func (b *Bine) Run(ctx context.Context, name string, args []string, streams IOStreams) error {
	bin, err := b.load(name)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	path, err := b.install(ctx, bin)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	err = run(ctx, path, args, streams)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

func (b *Bine) syncBins(ctx context.Context, bins []*bin) error {
	for _, item := range bins {
		bin, err := b.load(item.Name)
		if err != nil {
			return fmt.Errorf("sync: %v", err)
		}

		_, err = b.install(ctx, bin)
		if err != nil {
			return fmt.Errorf("sync: %v", err)
		}
	}

	return nil
}

// Sync installs all binaries defined in the configuration.
func (b *Bine) Sync(ctx context.Context) error {
	return b.syncBins(ctx, b.config.Bins)
}

func (b *Bine) Upgrade(ctx context.Context) ([]*ListItem, error) {
	return b.upgradeBins(ctx, b.config.Bins)
}

// UpgradeOne upgrades a single binary defined in the configuration.
func (b *Bine) UpgradeOne(ctx context.Context, name string) ([]*ListItem, error) {
	selected, err := b.load(name)
	if err != nil {
		return nil, fmt.Errorf("upgrade: %v", err)
	}

	return b.upgradeBins(ctx, []*bin{selected})
}

// ListOne returns list information for a single binary defined in the configuration.
func (b *Bine) ListOne(ctx context.Context, name string, installedOnly, outdatedOnly bool) ([]*ListItem, error) {
	selected, err := b.load(name)
	if err != nil {
		return nil, fmt.Errorf("list: %v", err)
	}

	return b.listBins(ctx, []*bin{selected}, installedOnly, outdatedOnly)
}

func (b *Bine) upgradeBins(ctx context.Context, bins []*bin) ([]*ListItem, error) {
	updates, err := b.listBins(ctx, bins, false, true)
	if err != nil {
		return nil, err
	}

	// Halt if any binary has an outdated check error.
	for _, item := range updates {
		if item.OutdatedCheckError != "" {
			return updates, errors.New("outdated check error")
		}
	}

	if len(updates) > 0 {
		if err := b.config.update(updates); err != nil {
			return nil, err
		}

		// For "latest" bins, config.update() does not modify the version in the
		// config file. Reinstall them explicitly so Sync() does not depend on
		// marker deletion as an implicit signal.
		for _, item := range updates {
			for _, bin := range b.config.Bins {
				if bin.Name == item.Name && bin.isLatest() {
					if err := b.reinstall(ctx, bin); err != nil {
						return updates, fmt.Errorf("reinstall latest-tracking bin %q: %v", bin.Name, err)
					}
				}
			}
		}
	}

	if err := b.syncBins(ctx, bins); err != nil {
		return updates, err
	}

	return updates, nil
}

type ListItem struct {
	Name string `json:"name"`
	// Prefixed with "v" if it's a semver.
	Version string `json:"version"`
	// Prefixed with "v" if it's a semver.
	Latest             string `json:"latest,omitempty"`
	OutdatedCheckError string `json:"outdated_check_error,omitempty"`
}

func (b *Bine) List(ctx context.Context, installedOnly, outdatedOnly bool) ([]*ListItem, error) {
	return b.listBins(ctx, b.config.Bins, installedOnly, outdatedOnly)
}

func (b *Bine) listBins(ctx context.Context, bins []*bin, installedOnly, outdatedOnly bool) ([]*ListItem, error) {
	var items []*ListItem

	for _, bin := range bins {
		if installedOnly {
			ok, err := b.installed(ctx, bin)
			if err != nil {
				return nil, fmt.Errorf("list: %v", err)
			} else if !ok {
				continue
			}
		}

		// For "latest" bins, read the resolved version from the marker so we
		// can display the actual installed version and compare it with the
		// upstream latest.
		resolvedVersion := ""
		latestInstalled := false
		var latestResolvedError error
		if bin.isLatest() {
			resolvedVersion, latestInstalled, latestResolvedError = b.latestResolvedVersion(ctx, bin)
		}

		var latestVersion string
		var outdatedCheckError string
		if outdatedOnly {
			if bin.isLatest() {
				if !latestInstalled {
					continue
				}
				if latestResolvedError != nil {
					outdatedCheckError = latestResolvedError.Error()
				}
			}

			var outdated bool
			var err error
			if outdatedCheckError == "" {
				outdated, latestVersion, err = bin.checkOutdated(ctx, resolvedVersion)
			}
			if err != nil {
				outdatedCheckError = err.Error()
			} else if outdatedCheckError == "" && !outdated {
				continue
			}
		}

		// Append the latest version with "v" prefix if it's a semver.
		if ver := semver.Canonical("v" + strings.TrimPrefix(latestVersion, "v")); ver != "" {
			latestVersion = "v" + latestVersion
		}

		// Display version: for "latest" bins show the resolved version if known.
		version := bin.usableVersion()
		if bin.isLatest() && resolvedVersion != "" {
			version = "v" + resolvedVersion
		}

		items = append(items, &ListItem{
			Name:               bin.Name,
			Version:            version,
			Latest:             latestVersion,
			OutdatedCheckError: outdatedCheckError,
		})
	}

	return items, nil
}

// clientLogger is a custom logger for the retryablehttp client.
type clientLogger struct {
	logger logr.Logger
}

var _ retryablehttp.LeveledLogger = clientLogger{}

func (l clientLogger) Error(msg string, keysAndValues ...any) {
	l.logger.V(0).Info(msg, keysAndValues...)
}

func (l clientLogger) Warn(msg string, keysAndValues ...any) {
	l.logger.V(0).Info(msg, keysAndValues...)
}

func (l clientLogger) Info(msg string, keysAndValues ...any) {
	l.logger.V(1).Info(msg, keysAndValues...)
}

func (l clientLogger) Debug(msg string, keysAndValues ...any) {
	l.logger.V(2).Info(msg, keysAndValues...)
}

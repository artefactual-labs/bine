package bine

import (
	"context"
	"crypto"
	"encoding/json"
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

	CacheDir    string // e.g. ~/.cache/bine/project/linux/amd64/
	BinDir      string // e.g. ~/.cache/bine/project/linux/amd64/bin/
	VersionsDir string // e.g. ~/.cache/bine/project/linux/amd64/versions/
}

// New creates a new Bine instance with default options.
func New() (*Bine, error) {
	return newBine(nil)
}

// NewWithOptions creates a new Bine instance with the given options.
func NewWithOptions(opts ...Option) (*Bine, error) {
	optsConfig := options{}
	for _, opt := range opts {
		if err := opt(&optsConfig); err != nil {
			return nil, err
		}
	}

	return newBine(&optsConfig)
}

// Option configures a Bine instance (used by NewWithOptions).
type Option func(*options) error

type options struct {
	logger       *logr.Logger
	cacheDirBase string
	ghAPIToken   string
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
func newBine(optsConfig *options) (*Bine, error) {
	if optsConfig == nil {
		optsConfig = &options{}
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 3
	stdClient := client.StandardClient()

	config, err := loadConfig(stdClient, optsConfig.ghAPIToken)
	if err != nil {
		return nil, err
	}

	b := &Bine{
		client: stdClient,
		config: config,
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
func (b *Bine) install(ctx context.Context, bin *bin) (string, error) {
	binPath := filepath.Join(b.BinDir, bin.Name)

	// If version marker exists, assume binary is already installed.
	if ok, err := b.installed(bin); ok {
		return binPath, nil
	} else if err != nil {
		return "", fmt.Errorf("failed to check if binary is installed: %v", err)
	}

	// Ensure the bin directory exists.
	if err := os.MkdirAll(b.BinDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %v", err)
	}

	if bin.goPkg() {
		if err := goInstall(ctx, bin, b.BinDir); err != nil {
			return "", fmt.Errorf("failed to install Go tool: %v", err)
		}
	} else {
		if err := binInstall(ctx, b.client, bin, binPath); err != nil {
			return "", fmt.Errorf("failed to install binary: %v", err)
		}
	}

	if err := b.markVersion(bin); err != nil {
		return "", err
	}

	return binPath, nil
}

// installed determines if a binary is already installed.
func (b *Bine) installed(bin *bin) (bool, error) {
	binPath := filepath.Join(b.BinDir, bin.Name)
	if info, err := os.Stat(binPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else if info.IsDir() {
		return false, fmt.Errorf("expected %q to be a file, but it's a directory", binPath)
	}

	versionMarker := filepath.Join(b.VersionsDir, bin.Name, bin.Version)
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
	return c.Algorithm == crypto.SHA256.String() || c.Value == sum
}

type versionMarkerDocument struct {
	Checksum versionMarkerChecksum `json:"checksum"`
}

// markVersion creates a version marker file for the binary.
func (b *Bine) markVersion(bin *bin) error {
	versionsDir := filepath.Join(b.VersionsDir, bin.Name)
	versionMarker := filepath.Join(versionsDir, bin.Version)

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
	}
	data, err := json.MarshalIndent(doc, "", "\t")
	if err != nil {
		return fmt.Errorf("json marshal: %v", err)
	}

	return renameio.WriteFile(versionMarker, data, 0o640, renameio.WithStaticPermissions(0o640))
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

// Sync installs all binaries defined in the configuration.
func (b *Bine) Sync(ctx context.Context) error {
	for _, item := range b.config.Bins {
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

func (b *Bine) Upgrade(ctx context.Context) ([]*ListItem, error) {
	updates, err := b.List(ctx, false, true)
	if err != nil {
		return nil, err
	}

	if len(updates) > 0 {
		if err := b.config.update(updates); err != nil {
			return nil, err
		}
	}

	if err := b.Sync(ctx); err != nil {
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
	var items []*ListItem

	for _, bin := range b.config.Bins {
		if installedOnly {
			ok, err := b.installed(bin)
			if err != nil {
				return nil, fmt.Errorf("list: %v", err)
			} else if !ok {
				continue
			}
		}

		var latestVersion string
		var outdatedCheckError string
		if outdatedOnly {
			var outdated bool
			var err error
			outdated, latestVersion, err = bin.checkOutdated(ctx)
			if err != nil {
				outdatedCheckError = err.Error()
			} else if !outdated {
				continue
			}
		}

		// Append the latest version with "v" prefix if it's a semver.
		if ver := semver.Canonical("v" + strings.TrimPrefix(latestVersion, "v")); ver != "" {
			latestVersion = "v" + latestVersion
		}

		items = append(items, &ListItem{
			Name:               bin.Name,
			Version:            bin.usableVersion(),
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

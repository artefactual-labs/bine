package bine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	cacheDirName = "bine"
	userAgent    = "bine"
)

type Bine struct {
	logger logr.Logger
	client *http.Client
	config *config

	CacheDir string // e.g. ~/.cache/bine/project/linux/amd64/
	BinDir   string // e.g. ~/.cache/bine/project/linux/amd64/bin/
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
	client := retryablehttp.NewClient()
	client.RetryMax = 3
	stdClient := client.StandardClient()

	config, err := loadConfig(stdClient, optsConfig.ghAPIToken)
	if err != nil {
		return nil, err
	}

	if optsConfig == nil {
		optsConfig = &options{}
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
	}

	return b, nil
}

// cacheDir returns the cache directory for the given project. Only called once
// at startup.
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
		return nil, fmt.Errorf("load: binary not found")
	}

	return bin, nil
}

// install a binary given its config.
func (b *Bine) install(ctx context.Context, bin *bin) (string, error) {
	path, err := ensureInstalled(ctx, b.client, bin, b.CacheDir)
	if err != nil {
		return "", fmt.Errorf("install: %v", err)
	}

	return path, nil
}

// installed checks if a binary is already installed.
func (b *Bine) installed(name string) (bool, error) {
	bin, err := b.load(name)
	if err != nil {
		return false, fmt.Errorf("installed: %v", err)
	}

	ok := installed(bin, b.CacheDir)

	return ok, nil
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

type ListItem struct {
	Name               string `json:"name"`
	Version            string `json:"version"`
	Latest             string `json:"latest,omitempty"`
	OutdatedCheckError string `json:"outdated_check_error,omitempty"`
}

func (b *Bine) List(ctx context.Context, installedOnly, outdatedOnly bool) ([]*ListItem, error) {
	var items []*ListItem

	for _, bin := range b.config.Bins {
		if installedOnly {
			ok, err := b.installed(bin.Name)
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

		items = append(items, &ListItem{
			Name:               bin.Name,
			Version:            bin.Version,
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

package bine

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/go-retryablehttp"
)

type Bine struct {
	CacheDir string // e.g. ~/.cache/bine/project/linux/amd64/
	BinDir   string // e.g. ~/.cache/bine/project/linux/amd64/bin/

	client *retryablehttp.Client
	config *config
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
	cacheDirBase string
}

// WithCacheDir specifies a custom base directory for the bine cache.
func WithCacheDir(path string) Option {
	return func(o *options) error {
		o.cacheDirBase = path
		return nil
	}
}

func newBine(optsConfig *options) (*Bine, error) {
	client := retryablehttp.NewClient()
	client.Logger = nil
	client.RetryMax = 3

	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	if optsConfig == nil {
		optsConfig = &options{}
	}

	cache, err := cacheDir(optsConfig.cacheDirBase, config.Project)
	if err != nil {
		return nil, err
	}

	return &Bine{
		CacheDir: cache,
		BinDir:   filepath.Join(cache, "bin"),
		client:   client,
		config:   config,
	}, nil
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
	path, err := ensureInstalled(ctx, b.client.StandardClient(), bin, b.CacheDir)
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
	Name    string `json:"name"`
	Version string `json:"version"`
	Latest  string `json:"latest,omitempty"`
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
		if outdatedOnly {
			var outdated bool
			var err error
			outdated, latestVersion, err = checkOutdated(ctx, bin, b.client.StandardClient())
			if err != nil {
				return nil, fmt.Errorf("list: %v", err)
			}
			if !outdated {
				continue
			}
		}

		items = append(items, &ListItem{
			Name:    bin.Name,
			Version: bin.Version,
			Latest:  latestVersion,
		})
	}

	return items, nil
}

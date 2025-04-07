package bine

import (
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

func New() (*Bine, error) {
	client := retryablehttp.NewClient()
	client.Logger = nil
	client.RetryMax = 3

	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	cacheDir, err := cacheDir(config.Project)
	if err != nil {
		return nil, err
	}

	return &Bine{
		CacheDir: cacheDir,
		BinDir:   filepath.Join(cacheDir, "bin"),
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
func (b *Bine) install(bin *bin) (string, error) {
	path, err := ensureInstalled(b.client.StandardClient(), bin, b.CacheDir)
	if err != nil {
		return "", fmt.Errorf("install: %v", err)
	}

	return path, nil
}

func (b *Bine) Get(name string) (string, error) {
	bin, err := b.load(name)
	if err != nil {
		return "", fmt.Errorf("get: %v", err)
	}

	path, err := b.install(bin)
	if err != nil {
		return "", fmt.Errorf("get: %v", err)
	}

	return path, nil
}

func (b *Bine) Run(name string, args []string, streams IOStreams) error {
	bin, err := b.load(name)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	path, err := b.install(bin)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	err = run(path, args, streams)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

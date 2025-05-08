package bine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tailscale/hujson"
)

type config struct {
	Project string `json:"project"`
	Bins    []*bin `json:"bins"`
}

func loadConfig(client *http.Client, ghAPIToken string) (*config, error) {
	curDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}

	configPath := ""
	searchDir := curDir
	for {
		p := filepath.Join(searchDir, ".bine.json")
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}

		parentDir := filepath.Dir(searchDir)
		if parentDir == searchDir {
			break // Reached the root directory.
		}
		searchDir = parentDir
	}

	if configPath == "" {
		return nil, errors.New("configuration file .bine.json not found")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %v", configPath, err)
	}

	cfg, err := unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal config %q: %v", configPath, err)
	}

	if namer, err := loadNamer(); err != nil {
		return nil, fmt.Errorf("load config namer: %v", err)
	} else {
		namer.run(cfg)
	}

	if cfg.Project == "" {
		return nil, fmt.Errorf("project name is empty in config file %q", configPath)
	}

	for _, b := range cfg.Bins {
		if err := b.loadProvider(client, ghAPIToken); err != nil {
			return nil, fmt.Errorf("load provider for bin %q: %v", b.Name, err)
		}
		if b.canonicalVersion() == "" {
			return nil, fmt.Errorf("invalid version %q for binary %q: use semver", b.Version, b.Name)
		}
	}

	return cfg, nil
}

func unmarshal(b []byte) (*config, error) {
	b, err := hujson.Standardize(b)
	if err != nil {
		return nil, err
	}

	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	return &c, nil
}

// namer computes the asset names defined in the configuration.
type namer struct {
	unameOS   string // `uname -s`, e.g. "Linux", "Darwin"...
	unameArch string // `uname -m`, e.g. "x86_64", "arm64"...
}

func loadNamer() (*namer, error) {
	n := namer{}

	out, err := exec.Command("uname", "-s").Output()
	if err != nil {
		return nil, fmt.Errorf("uname: %v", err)
	}
	n.unameOS = strings.TrimSpace(string(out))

	out, err = exec.Command("uname", "-m").Output()
	if err != nil {
		return nil, fmt.Errorf("uname: %v", err)
	}
	n.unameArch = strings.TrimSpace(string(out))

	return &n, nil
}

func (n *namer) run(c *config) {
	for _, b := range c.Bins {
		if b.goPkg() {
			continue
		}
		asset := b.AssetPattern
		asset = strings.ReplaceAll(asset, "{name}", b.Name)
		asset = strings.ReplaceAll(asset, "{version}", b.Version)
		asset = strings.ReplaceAll(asset, "{goos}", runtime.GOOS)
		asset = strings.ReplaceAll(asset, "{goarch}", runtime.GOARCH)
		asset = strings.ReplaceAll(asset, "{os}", n.unameOS)
		asset = strings.ReplaceAll(asset, "{arch}", n.unameArch)

		b.asset = asset
	}
}

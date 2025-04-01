package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type config struct {
	Project string `json:"project"`
	Bins    []*bin `json:"bins"`
}

func loadConfig() (*config, error) {
	curDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	namer, err := loadNamer()
	if err != nil {
		return nil, err
	}

	for {
		configPath := filepath.Join(curDir, ".bine.json")
		if _, err := os.Stat(configPath); err == nil {
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, err
			}
			var cfg config
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, err
			}
			namer.run(&cfg)
			return &cfg, nil
		}

		parentDir := filepath.Dir(curDir)
		if parentDir == curDir {
			break
		}
		curDir = parentDir
	}

	return nil, errors.New("configuration file .bine.json not found")
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

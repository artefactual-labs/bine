package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type config struct {
	Project string `json:"project"`
	Bins    []bin  `json:"bins"`
}

func loadConfig() (*config, error) {
	curDir, err := os.Getwd()
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

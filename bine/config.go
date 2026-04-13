package bine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
)

type configFormat string

const (
	configFormatJSON configFormat = "json"
	configFormatTOML configFormat = "toml"
)

type configFile struct {
	path   string
	format configFormat
}

type config struct {
	Project string `json:"project" toml:"project"`
	Bins    []*bin `json:"bins" toml:"bins"`

	// path to the configuration file on disk, used during the update process.
	path string

	// format of the configuration file on disk, used during the update process.
	format configFormat

	// namer is used to compute the asset names. This is set when the config
	// is loaded and during the update process.
	namer *namer
}

// loadConfig loads the configuration file from the current working directory
// or its parent directories.
func loadConfig(ctx context.Context, client *http.Client, ghAPIToken string) (*config, error) {
	curDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}

	configFile, err := findConfigFile(curDir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configFile.path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %v", configFile.path, err)
	}

	cfg, err := unmarshalConfig(configFile.format, data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal config %q: %v", configFile.path, err)
	}
	cfg.path = configFile.path
	cfg.format = configFile.format

	if cfg.Project == "" {
		return nil, fmt.Errorf("project name is empty in config file %q", configFile.path)
	}

	if namer, err := createNamer(ctx); err != nil {
		return nil, fmt.Errorf("load config namer: %v", err)
	} else {
		cfg.namer = namer
		cfg.namer.run(cfg.Bins)
	}

	for _, b := range cfg.Bins {
		if err := b.loadProvider(client, ghAPIToken); err != nil {
			return nil, fmt.Errorf("load provider for bin %q: %v", b.Name, err)
		}
	}

	return cfg, nil
}

// update applies the updates to the configuration file when the format supports
// in-place edits.
func (c *config) update(updates []*ListItem) error {
	if c.path == "" {
		return errors.New("config path is not set")
	}

	changes := map[string]string{}
	for _, item := range updates {
		for _, b := range c.Bins {
			if b.Name == item.Name {
				// "latest" bins are reinstalled without changing the config.
				if b.isLatest() {
					break
				}
				nextVersion := strings.TrimPrefix(item.Latest, "v")
				if nextVersion != "" && b.Version != nextVersion {
					changes[b.Name] = nextVersion
				}
				break
			}
		}
	}
	if len(changes) == 0 {
		return nil
	}

	if err := updateConfigFile(c.path, c.format, changes); err != nil {
		return err
	}

	for _, b := range c.Bins {
		if nextVersion, ok := changes[b.Name]; ok {
			b.Version = nextVersion
		}
	}
	c.namer.run(c.Bins)
	return nil
}

func findConfigFile(startDir string) (*configFile, error) {
	searchDir := startDir
	for {
		jsonPath := filepath.Join(searchDir, ".bine.json")
		tomlPath := filepath.Join(searchDir, ".bine.toml")

		jsonExists, err := configFileExists(jsonPath)
		if err != nil {
			return nil, fmt.Errorf("stat config %q: %w", jsonPath, err)
		}
		tomlExists, err := configFileExists(tomlPath)
		if err != nil {
			return nil, fmt.Errorf("stat config %q: %w", tomlPath, err)
		}

		switch {
		case jsonExists && tomlExists:
			return nil, fmt.Errorf("configuration files %q and %q both exist; keep only one", jsonPath, tomlPath)
		case jsonExists:
			return &configFile{path: jsonPath, format: configFormatJSON}, nil
		case tomlExists:
			return &configFile{path: tomlPath, format: configFormatTOML}, nil
		}

		parentDir := filepath.Dir(searchDir)
		if parentDir == searchDir {
			break
		}
		searchDir = parentDir
	}

	return nil, errors.New("configuration file .bine.json or .bine.toml not found")
}

func configFileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

func unmarshalConfig(format configFormat, b []byte) (*config, error) {
	switch format {
	case configFormatJSON:
		return unmarshalJSONConfig(b)
	case configFormatTOML:
		return unmarshalTOMLConfig(b)
	default:
		return nil, fmt.Errorf("unsupported config format %q", format)
	}
}

func unmarshalJSONConfig(b []byte) (*config, error) {
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

func unmarshalTOMLConfig(b []byte) (*config, error) {
	var c config
	if err := toml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func updateConfigFile(path string, format configFormat, changes map[string]string) error {
	switch format {
	case configFormatJSON:
		return updateJSONConfigFile(path, changes)
	case configFormatTOML:
		return errors.New("upgrade is not supported for TOML config files; use .bine.json or update versions manually")
	default:
		return fmt.Errorf("unsupported config format %q", format)
	}
}

func updateJSONConfigFile(path string, changes map[string]string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}
	defer f.Close()

	contents, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read file: %v", err)
	}

	tree, err := hujson.Parse(contents)
	if err != nil {
		return fmt.Errorf("hujson parse: %v", err)
	}

	// Modify the version attribute using JSON Patch.
	for i := 0; ; i++ {
		if binNode := tree.Find(fmt.Sprintf("/bins/%d", i)); binNode == nil {
			break
		} else if nameNode := binNode.Find("/name"); nameNode == nil {
			continue
		} else if nameLiteral, ok := nameNode.Value.(hujson.Literal); !ok {
			continue
		} else if latest, ok := changes[nameLiteral.String()]; !ok {
			continue
		} else if err := binNode.Patch(fmt.Appendf(nil, `[{"op": "replace", "path": "/version", "value": "%s"}]`, latest)); err != nil {
			return fmt.Errorf("patch replace: %v", err)
		}
	}

	// Write the modified tree back to the file and truncate it to the new size.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek file: %v", err)
	}
	blob := tree.Pack()
	if _, err := f.Write(blob); err != nil {
		return fmt.Errorf("write file: %v", err)
	}
	if err := f.Truncate(int64(len(blob))); err != nil {
		return fmt.Errorf("truncate file: %v", err)
	}

	return f.Sync()
}

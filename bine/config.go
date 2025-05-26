package bine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

type config struct {
	Project string `json:"project"`
	Bins    []*bin `json:"bins"`

	// path to the configuration file on disk, used during the update process.
	path string

	// namer is used to compute the asset names. This is set when the config
	// is loaded and during the update process.
	namer *namer
}

// loadConfig loads the configuration file from the current working directory
// or its parent directories.
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
	cfg.path = configPath

	if cfg.Project == "" {
		return nil, fmt.Errorf("project name is empty in config file %q", configPath)
	}

	if namer, err := createNamer(); err != nil {
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

// update applies the updates to the configuration file. It respects the
// original formatting of the file, including comments and whitespace.
func (c *config) update(updates []*ListItem) error {
	if c.path == "" {
		return errors.New("config path is not set")
	}

	changes := map[string]string{}
	for _, item := range updates {
		for _, b := range c.Bins {
			if b.Name == item.Name {
				if item.Latest != "" && b.Version != item.Latest {
					b.Version = item.Latest
					changes[b.Name] = strings.TrimPrefix(item.Latest, "v")
				}
				break
			}
		}
	}
	if len(changes) == 0 {
		return nil
	}

	c.namer.run(c.Bins)

	f, err := os.OpenFile(c.path, os.O_RDWR, 0)
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

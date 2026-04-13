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
	"slices"
	"strconv"
	"strings"

	"github.com/google/renameio/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
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
	applyLibraryDefaults(cfg)

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
		return updateTOMLConfigFile(path, changes)
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

type tomlBinTable struct {
	name         string
	versionRange unstable.Range
	versionRaw   []byte
	directKeys   bool
}

type tomlReplacement struct {
	start int
	end   int
	text  string
}

func updateTOMLConfigFile(path string, changes map[string]string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat file: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %v", err)
	}

	parser := unstable.Parser{KeepComments: true}
	parser.Reset(contents)

	var (
		current              *tomlBinTable
		replacements         []tomlReplacement
		applied              = map[string]bool{}
		sawUnsupportedLayout bool
	)

	flush := func() error {
		if current == nil {
			return nil
		}
		defer func() { current = nil }()

		nextVersion, ok := changes[current.name]
		if !ok {
			return nil
		}
		if len(current.versionRaw) == 0 {
			return fmt.Errorf("upgrade is only supported for TOML [[bins]] tables with explicit string version keys; %q is missing one", current.name)
		}

		versionLiteral, err := tomlStringLiteral(nextVersion, current.versionRaw)
		if err != nil {
			return fmt.Errorf("upgrade TOML version for %q: %v", current.name, err)
		}

		start := int(current.versionRange.Offset)
		end := start + int(current.versionRange.Length)
		replacements = append(replacements, tomlReplacement{
			start: start,
			end:   end,
			text:  versionLiteral,
		})
		applied[current.name] = true
		return nil
	}

	for parser.NextExpression() {
		expr := parser.Expression()
		switch expr.Kind {
		case unstable.Comment:
			continue
		case unstable.ArrayTable:
			if err := flush(); err != nil {
				return err
			}

			if path := tomlKeyPath(expr); len(path) == 1 && path[0] == "bins" {
				current = &tomlBinTable{directKeys: true}
			} else {
				current = nil
			}
		case unstable.Table:
			path := tomlKeyPath(expr)
			if current != nil && len(path) > 0 && path[0] == "bins" {
				current.directKeys = false
				continue
			}
			if err := flush(); err != nil {
				return err
			}
		case unstable.KeyValue:
			keyPath := tomlKeyPath(expr)
			if len(keyPath) == 1 && keyPath[0] == "bins" {
				sawUnsupportedLayout = true
			}
			if current == nil || !current.directKeys || len(keyPath) != 1 {
				continue
			}

			switch keyPath[0] {
			case "name":
				if expr.Value().Kind == unstable.String {
					current.name = string(expr.Value().Data)
				}
			case "version":
				if expr.Value().Kind != unstable.String {
					return fmt.Errorf("upgrade is only supported for TOML string version keys in [[bins]] tables")
				}
				current.versionRange = expr.Value().Raw
				current.versionRaw = slices.Clone(parser.Raw(expr.Value().Raw))
			}
		default:
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if parser.Error() != nil {
		return fmt.Errorf("parse TOML: %v", parser.Error())
	}
	if err := flush(); err != nil {
		return err
	}

	var missing []string
	for name := range changes {
		if !applied[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		if sawUnsupportedLayout {
			return fmt.Errorf("upgrade is only supported for TOML configs using [[bins]] tables; couldn't update %s", strings.Join(missing, ", "))
		}
		return fmt.Errorf("failed to locate TOML version keys for %s", strings.Join(missing, ", "))
	}

	slices.SortFunc(replacements, func(a, b tomlReplacement) int {
		return b.start - a.start
	})

	updated := contents
	for _, repl := range replacements {
		updated = append(updated[:repl.start], append([]byte(repl.text), updated[repl.end:]...)...)
	}

	perm := info.Mode().Perm()
	if err := renameio.WriteFile(path, updated, perm, renameio.WithStaticPermissions(perm)); err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	return nil
}

func tomlKeyPath(node *unstable.Node) []string {
	var path []string
	it := node.Key()
	for it.Next() {
		path = append(path, string(it.Node().Data))
	}
	return path
}

func tomlStringLiteral(value string, raw []byte) (string, error) {
	switch {
	case len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' && !strings.HasPrefix(string(raw), "'''"):
		if strings.Contains(value, "'") || strings.ContainsAny(value, "\r\n") {
			return "", errors.New("single-quoted TOML strings can't encode this value")
		}
		return "'" + value + "'", nil
	case len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' && !strings.HasPrefix(string(raw), `"""`):
		return strconv.Quote(value), nil
	default:
		return "", errors.New("unsupported TOML string literal form")
	}
}

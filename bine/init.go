package bine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Init creates an empty project configuration in the current directory.
// Empty project and format values default to the directory name and JSON.
// An existing JSON or TOML configuration is never overwritten.
func Init(project, format string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if project == "" {
		project = filepath.Base(dir)
	}
	if err := validateLocalName(project); err != nil {
		return "", fmt.Errorf("invalid project name: %w", err)
	}
	if format == "" {
		format = string(configFormatJSON)
	}
	if format != string(configFormatJSON) && format != string(configFormatTOML) {
		return "", fmt.Errorf("unsupported config format %q; use json or toml", format)
	}
	for _, ext := range []string{"json", "toml"} {
		path := filepath.Join(dir, ".bine."+ext)
		if _, err := os.Lstat(path); err == nil {
			return "", fmt.Errorf("configuration %q already exists", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}

	cfg := config{Project: project, Bins: []*bin{}}
	var data []byte
	if format == string(configFormatJSON) {
		data, err = json.MarshalIndent(cfg, "", "  ")
		data = append(data, '\n')
	} else {
		data, err = toml.Marshal(cfg)
	}
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, ".bine."+format)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func validateLocalName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("name must be a single, nonempty path component")
	}
	return nil
}

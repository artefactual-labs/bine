package bine

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
)

func TestConfigUpdate(t *testing.T) {
	configDoc := `{
    "project": "test",
    "bins": [
        // perpignan is so cool.
        {
            "name": "perpignan",
            "url": "https://github.com/sevein/perpignan",
            "version": "1.0.0",
            "asset_pattern": "{name}_{version}_{goos}_{goarch}"
        },
        // go-mod-outdated finds outdated deps.
        {
            "name": "go-mod-outdated",
            "url": "https://github.com/psampaz/go-mod-outdated",
            "version": "0.9.0",
            "asset_pattern": "{name}_{version}_{os}_{arch}.tar.gz"
        },
    ]
}`

	t.Run("Rejects empty config", func(t *testing.T) {
		cfg := &config{}

		err := cfg.update([]*ListItem{{Name: "perpignan", Latest: "1.1.0"}})
		assert.Error(t, err, "config path is not set")
	})

	t.Run("Rejects irrelevant updates", func(t *testing.T) {
		cfg := &config{path: "/tmp/.bine.json"}

		err := cfg.update([]*ListItem{{Name: "irrelevant", Latest: "1.1.0"}})
		assert.NilError(t, err)
	})

	t.Run("Rejects config with path wrong", func(t *testing.T) {
		cfg := &config{
			Project: "project",
			Bins: []*bin{
				{Name: "perpignan", Version: "1.0.0"},
			},
			path: "/tmp/.bine.12345.json",
		}

		err := cfg.update([]*ListItem{{Name: "perpignan", Latest: "1.1.0"}})
		assert.Error(t, err, "open file: open /tmp/.bine.12345.json: no such file or directory")
	})

	t.Run("Modifies the configuration successfully", func(t *testing.T) {
		tmpDir := fs.NewDir(t, "bine", fs.WithFile(".bine.json", configDoc))

		t.Chdir(tmpDir.Path())
		cfg, err := loadConfig(nil, "")
		assert.NilError(t, err)

		err = cfg.update([]*ListItem{{Name: "perpignan", Latest: "1.1.0"}})
		assert.NilError(t, err)

		// Check that the config state was modified.
		assert.DeepEqual(t,
			cfg.Bins,
			[]*bin{
				{Name: "perpignan", Version: "1.1.0"},
				{Name: "go-mod-outdated", Version: "0.9.0"},
			},
			cmpopts.IgnoreFields(bin{}, "URL", "AssetPattern"),
			cmpopts.IgnoreUnexported(bin{}),
		)

		// Check that the config file was modified.
		contents, err := os.ReadFile(tmpDir.Join(".bine.json"))
		assert.NilError(t, err)
		assert.DeepEqual(t, contents, []byte(`{
    "project": "test",
    "bins": [
        // perpignan is so cool.
        {
            "name": "perpignan",
            "url": "https://github.com/sevein/perpignan",
            "version": "1.1.0",
            "asset_pattern": "{name}_{version}_{goos}_{goarch}"
        },
        // go-mod-outdated finds outdated deps.
        {
            "name": "go-mod-outdated",
            "url": "https://github.com/psampaz/go-mod-outdated",
            "version": "0.9.0",
            "asset_pattern": "{name}_{version}_{os}_{arch}.tar.gz"
        },
    ]
}`))
	})
}

package bine

import (
	"os"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
)

func modifyRuntime(t *testing.T, newGOOS, newGOARCH string) {
	t.Helper()

	goos = newGOOS
	goarch = newGOARCH

	t.Cleanup(func() {
		goos = runtime.GOOS
		goarch = runtime.GOARCH
	})
}

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
		cfg, err := loadConfig(t.Context(), nil, "")
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

func TestConfigModifiers(t *testing.T) {
	t.Run("Applies modifiers correctly", func(t *testing.T) {
		tmpDir := fs.NewDir(t, "bine", fs.WithFile(".bine.json", `{
    "project": "test",
    "bins": [
        {
            "name": "grpcurl",
            "url": "https://github.com/fullstorydev/grpcurl",
            "version": "1.9.3",
            "asset_pattern": "{name}_{version}_{goos}_{goarch}.tar.gz",
            "modifiers": {
                "goos": {
                    "darwin": "osx"
                },
                "goarch": {
                    "amd64": "x86_64"
                }
            }
        },
        {
            "name": "perpignan",
            "url": "https://github.com/sevein/perpignan",
            "version": "1.0.0",
            "asset_pattern": "{name}_{version}_{goos}_{goarch}"
        },
    ]
}`))
		t.Chdir(tmpDir.Path())

		modifyRuntime(t, "darwin", "arm64")

		cfg, err := loadConfig(t.Context(), nil, "")
		assert.NilError(t, err)

		// grpcurl leverages the modifiers.
		bin := cfg.Bins[0]
		{
			url, err := bin.provider.downloadURL(bin)
			assert.NilError(t, err)
			assert.Equal(t, url, "https://github.com/fullstorydev/grpcurl/releases/download/v1.9.3/grpcurl_1.9.3_osx_arm64.tar.gz")
		}

		// perpignan still works without modifiers.
		bin = cfg.Bins[1]
		{
			url, err := bin.provider.downloadURL(bin)
			assert.NilError(t, err)
			assert.Equal(t, url, "https://github.com/sevein/perpignan/releases/download/v1.0.0/perpignan_1.0.0_darwin_arm64")
		}
	})
}

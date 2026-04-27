package bine

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
)

func TestApplyLibraryDefaults(t *testing.T) {
	t.Run("fills known asset bin fields by URL", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:    "go-mod-outdated",
					URL:     "https://github.com/psampaz/go-mod-outdated",
					Version: "0.9.0",
				},
			},
		}

		applyLibraryDefaults(cfg)

		b := cfg.Bins[0]
		assert.Equal(t, b.URL, "https://github.com/psampaz/go-mod-outdated")
		assert.Equal(t, b.AssetPattern, "{name}_{version}_{os}_{arch}.tar.gz")
		assert.Equal(t, b.GoPackage, "")
	})

	t.Run("does not use name defaults", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:    "go-mod-outdated",
					Version: "0.9.0",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].URL, "")
		assert.Equal(t, cfg.Bins[0].AssetPattern, "")
	})

	t.Run("fills by known URL with a custom name", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:    "lint",
					URL:     "https://github.com/golangci/golangci-lint",
					Version: "2.11.2",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].AssetPattern, "{name}-{version}-{goos}-{goarch}.tar.gz")
	})

	t.Run("does not use name defaults when an explicit URL points elsewhere", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:    "golangci-lint",
					URL:     "https://github.com/example/fork",
					Version: "2.11.2",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].AssetPattern, "")
	})

	t.Run("does not use name defaults when an explicit Go package points elsewhere", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:      "workflowcheck",
					GoPackage: "example.com/workflowcheck",
					Version:   "0.4.0",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].URL, "")
		assert.Equal(t, cfg.Bins[0].GoPackage, "example.com/workflowcheck")
	})

	t.Run("fills known tag pattern and modifiers", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:    "jq",
					URL:     "https://github.com/jqlang/jq",
					Version: "1.8.1",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].AssetPattern, "{name}-{goos}-{goarch}")
		assert.Equal(t, cfg.Bins[0].TagPattern, "{name}-{version}")
		assert.Equal(t, cfg.Bins[0].Modifiers["goos"]["darwin"], "macos")
	})

	t.Run("preserves user fields", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:         "go-mod-outdated",
					URL:          "https://github.com/example/fork",
					AssetPattern: "custom-{version}",
					TagPattern:   "release-{version}",
					GoPackage:    "example.com/tool",
					Version:      "0.9.0",
				},
			},
		}

		applyLibraryDefaults(cfg)

		b := cfg.Bins[0]
		assert.Equal(t, b.URL, "https://github.com/example/fork")
		assert.Equal(t, b.AssetPattern, "custom-{version}")
		assert.Equal(t, b.TagPattern, "release-{version}")
		assert.Equal(t, b.GoPackage, "example.com/tool")
	})

	t.Run("merges modifiers without overwriting user entries", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name: "shellcheck",
					URL:  "https://github.com/koalaman/shellcheck",
					Modifiers: map[string]map[string]string{
						"goarch": {
							"amd64": "custom-amd64",
						},
					},
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].Modifiers["goarch"]["amd64"], "custom-amd64")
		assert.Equal(t, cfg.Bins[0].Modifiers["goarch"]["arm64"], "aarch64")
	})

	t.Run("fills modifiers", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name: "grpcurl",
					URL:  "https://github.com/fullstorydev/grpcurl",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].Modifiers["goos"]["darwin"], "osx")
		assert.Equal(t, cfg.Bins[0].Modifiers["goarch"]["386"], "x86_32")
		assert.Equal(t, cfg.Bins[0].Modifiers["goarch"]["amd64"], "x86_64")
	})

	t.Run("leaves unknown bins alone", func(t *testing.T) {
		cfg := &config{
			Bins: []*bin{
				{
					Name:    "custom",
					Version: "1.2.3",
				},
			},
		}

		applyLibraryDefaults(cfg)

		assert.Equal(t, cfg.Bins[0].URL, "")
		assert.Equal(t, cfg.Bins[0].AssetPattern, "")
		assert.Equal(t, cfg.Bins[0].GoPackage, "")
	})
}

func TestLoadConfigUsesLibraryDefaults(t *testing.T) {
	tmpDir := fs.NewDir(t, "bine", fs.WithFile(".bine.toml", `project = "test"

[[bins]]
name = "go-mod-outdated"
url = "https://github.com/psampaz/go-mod-outdated"
version = "0.9.0"

[[bins]]
name = "workflowcheck"
go_package = "go.temporal.io/sdk/contrib/tools/workflowcheck"
version = "0.4.0"
`))
	t.Chdir(tmpDir.Path())

	cfg, err := loadConfig(t.Context(), nil, "")
	assert.NilError(t, err)

	assert.Equal(t, cfg.Bins[0].URL, "https://github.com/psampaz/go-mod-outdated")
	assert.Equal(t, cfg.Bins[0].AssetPattern, "{name}_{version}_{os}_{arch}.tar.gz")
	assert.Assert(t, cfg.Bins[0].asset != "")
	assert.Equal(t, cfg.Bins[1].GoPackage, "go.temporal.io/sdk/contrib/tools/workflowcheck")
	assert.Equal(t, cfg.Bins[1].AssetPattern, "")
}

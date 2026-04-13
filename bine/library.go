package bine

type binTemplate struct {
	AssetPattern string
	TagPattern   string
	Modifiers    map[string]map[string]string
}

var knownAssetTemplates = map[string]binTemplate{
	"https://github.com/rhysd/actionlint": {
		AssetPattern: "{name}_{version}_{goos}_{goarch}.tar.gz",
	},
	"https://release.ariga.io/atlas": {
		AssetPattern: "atlas-{goos}-{goarch}-v{version}",
	},
	"https://github.com/bufbuild/buf": {
		AssetPattern: "{name}-{os}-{arch}",
	},
	"https://github.com/abice/go-enum": {
		AssetPattern: "{name}_{os}_{arch}",
	},
	"https://github.com/psampaz/go-mod-outdated": {
		AssetPattern: "{name}_{version}_{os}_{arch}.tar.gz",
	},
	"https://github.com/golangci/golangci-lint": {
		AssetPattern: "{name}-{version}-{goos}-{goarch}.tar.gz",
	},
	"https://github.com/goreleaser/goreleaser": {
		AssetPattern: "{name}_{os}_{arch}.tar.gz",
	},
	"https://github.com/gotestyourself/gotestsum": {
		AssetPattern: "{name}_{version}_{goos}_{goarch}.tar.gz",
	},
	"https://github.com/fullstorydev/grpcurl": {
		AssetPattern: "{name}_{version}_{goos}_{goarch}.tar.gz",
		Modifiers: map[string]map[string]string{
			"goos": {
				"darwin": "osx",
			},
			"goarch": {
				"386":   "x86_32",
				"amd64": "x86_64",
			},
		},
	},
	"https://github.com/gohugoio/hugo": {
		AssetPattern: "{name}_extended_{version}_{goos}-{goarch}.tar.gz",
	},
	"https://github.com/jqlang/jq": {
		AssetPattern: "{name}-{goos}-{goarch}",
	},
	"https://github.com/golang-migrate/migrate": {
		AssetPattern: "{name}.{goos}-{goarch}.tar.gz",
	},
	"https://github.com/koalaman/shellcheck": {
		AssetPattern: "{name}-v{version}.{goos}.{goarch}.tar.xz",
		Modifiers: map[string]map[string]string{
			"goarch": {
				"amd64": "x86_64",
				"arm64": "aarch64",
			},
		},
	},
	"https://github.com/mvdan/sh": {
		AssetPattern: "{name}_v{version}_{goos}_{goarch}",
	},
	"https://github.com/sqlc-dev/sqlc": {
		AssetPattern: "{name}_{version}_{goos}_{goarch}.tar.gz",
	},
	"https://github.com/temporalio/cli": {
		AssetPattern: "temporal_cli_{version}_{goos}_{goarch}.tar.gz",
	},
	"https://github.com/mfridman/tparse": {
		AssetPattern: "{name}_{goos}_{arch}",
	},
	"https://github.com/astral-sh/uv": {
		AssetPattern: "{name}-{triple}.tar.gz",
		TagPattern:   "{version}",
	},
}

func applyLibraryDefaults(cfg *config) {
	for _, b := range cfg.Bins {
		t, ok := knownAssetTemplates[b.URL]
		if !ok {
			continue
		}
		applyBinTemplate(b, t)
	}
}

func applyBinTemplate(b *bin, t binTemplate) {
	if b.AssetPattern == "" {
		b.AssetPattern = t.AssetPattern
	}
	if b.TagPattern == "" {
		b.TagPattern = t.TagPattern
	}
	if len(t.Modifiers) > 0 {
		b.Modifiers = mergeModifiers(b.Modifiers, t.Modifiers)
	}
}

func mergeModifiers(dst, src map[string]map[string]string) map[string]map[string]string {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]map[string]string, len(src))
	}
	for variable, replacements := range src {
		if dst[variable] == nil {
			dst[variable] = make(map[string]string, len(replacements))
		}
		for from, to := range replacements {
			if _, ok := dst[variable][from]; !ok {
				dst[variable][from] = to
			}
		}
	}
	return dst
}

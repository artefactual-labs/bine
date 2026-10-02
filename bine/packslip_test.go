package bine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/internal/packslip"
)

func TestPackslipConfig(t *testing.T) {
	for _, tt := range []struct {
		name, format, data string
	}{
		{"json", "json", `{"project":"test","bins":[
// The alias and its selection must survive version updates.
{"name":"alias","version":"1.2.3+build.1","packslip":{"project":"github.com/example/tool","command":"tool","variant":"baseline"}},
{"name":"other","version":"9.0.0","packslip":{"project":"github.com/example/other"}}
]}`},
		{"inline table", "toml", `project="test"
[[bins]]
name="alias"
version="1.2.3+build.1" # Keep this pin comment.
packslip={project="github.com/example/tool", command="tool", variant="baseline"}
[[bins]]
name="other"
version="9.0.0"
packslip={project="github.com/example/other"}
`},
		{"table", "toml", `project="test"
[[bins]]
name="alias"
version="1.2.3+build.1" # Keep this pin comment.
[bins.packslip]
project="github.com/example/tool"
command="tool" # Keep this selection comment.
variant="baseline"
[[bins]]
name="other"
version="9.0.0"
[bins.packslip]
project="github.com/example/other"
`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("PATH", "") // This source must not need uname or rustc at load time.
			assert.NilError(t, os.WriteFile(filepath.Join(dir, ".bine."+tt.format), []byte(tt.data), 0o600))
			cfg, err := loadConfig(t.Context(), sourceFactory{client: &http.Client{}})
			assert.NilError(t, err)
			assert.Equal(t, cfg.Bins[0].usableVersion(), "v1.2.3+build.1")
			assert.DeepEqual(t, cfg.Bins[0].packslipRequest(), packslip.Request{Project: "github.com/example/tool", Version: "1.2.3+build.1", Name: "alias", Command: "tool", Variant: "baseline"})
			assert.NilError(t, cfg.update([]*ListItem{{Name: "alias", Latest: "v1.2.4+build.2"}}))
			assert.Equal(t, cfg.Bins[0].Version, "1.2.4+build.2")
			updated, err := os.ReadFile(cfg.path)
			assert.NilError(t, err)
			assert.Equal(t, string(updated), strings.Replace(tt.data, "1.2.3+build.1", "1.2.4+build.2", 1))
			reloaded, err := loadConfig(t.Context(), sourceFactory{client: &http.Client{}})
			assert.NilError(t, err)
			assert.DeepEqual(t, reloaded.Bins[0].Packslip, cfg.Bins[0].Packslip)
			assert.Equal(t, reloaded.Bins[1].Version, "9.0.0")
			assert.Equal(t, reloaded.Bins[1].Packslip.Project, "github.com/example/other")
		})
	}
	for _, change := range []func(*bin){
		func(b *bin) { b.URL = "https://github.com/example/tool" },
		func(b *bin) { b.GoPackage = "example.com/tool" },
		func(b *bin) { b.AssetPattern = "foo" },
		func(b *bin) { b.TagPattern = "v{version}" },
		func(b *bin) { b.Modifiers = map[string]map[string]string{"os": {"darwin": "macos"}} },
		func(b *bin) { b.Name = "../tool" },
		func(b *bin) { b.Version = "latest" },
		func(b *bin) { b.Version = "1.2" },
		func(b *bin) { b.Packslip.Command = "../tool" },
		func(b *bin) { b.Packslip.Variant = "../variant" },
	} {
		b := &bin{Name: "tool", Version: "1.2.3", Packslip: &packslipConfig{Project: "github.com/example/tool"}}
		change(b)
		assert.Assert(t, b.validatePackslip() != nil)
	}
}

func TestPackslipConfigRejectsInvalidObjects(t *testing.T) {
	for _, tt := range []struct {
		name, json, toml, want string
	}{
		{"empty", `{}`, `{}`, "packslip.project is required"},
		{"missing project", `{"command":"tool"}`, `{command="tool"}`, "packslip.project is required"},
		{"scalar", `"github.com/example/tool"`, `"github.com/example/tool"`, ""},
		{"array", `[]`, `[]`, ""},
		{"command typo", `{"project":"github.com/example/tool","commmand":"tool"}`, `{project="github.com/example/tool", commmand="tool"}`, "commmand"},
		{"variant typo", `{"project":"github.com/example/tool","varaint":"baseline"}`, `{project="github.com/example/tool", varaint="baseline"}`, "varaint"},
		{"nested unknown", `{"project":"github.com/example/tool","options":{"variant":"baseline"}}`, `{project="github.com/example/tool", options={variant="baseline"}}`, "options"},
		{"invalid project type", `{"project":42}`, `{project=42}`, ""},
	} {
		for _, format := range []configFormat{configFormatJSON, configFormatTOML} {
			t.Run(tt.name+"/"+string(format), func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				data := `{"project":"test","bins":[{"name":"tool","version":"1.2.3","packslip":` + tt.json + `}]}`
				if format == configFormatTOML {
					data = "project='test'\n[[bins]]\nname='tool'\nversion='1.2.3'\npackslip=" + tt.toml + "\n"
				}
				assert.NilError(t, os.WriteFile(filepath.Join(dir, ".bine."+string(format)), []byte(data), 0o600))
				_, err := loadConfig(t.Context(), sourceFactory{client: &http.Client{}})
				assert.Assert(t, err != nil)
				if tt.want != "" {
					assert.ErrorContains(t, err, tt.want)
				}
			})
		}
	}
	for _, selection := range []string{
		"[bins.packslip]\nproject='github.com/example/tool'\nvaraint='baseline'\n",
		"[bins.packslip]\nproject='github.com/example/tool'\n[bins.packslip.options]\nvariant='baseline'\n",
	} {
		_, err := unmarshalTOMLConfig([]byte("project='test'\n[[bins]]\nname='tool'\nversion='1.2.3'\n" + selection))
		assert.ErrorContains(t, err, "unknown Packslip configuration field")
	}
}

func TestPackslipConfigPreservesLegacyExtensionFields(t *testing.T) {
	for _, tt := range []struct {
		format configFormat
		data   string
	}{
		{configFormatJSON, `{"project":"test","extra":true,"bins":[
{"name":"go-tool","go_package":"example.com/tool","command":{"custom":true},"variant":["custom"]},
{"name":"jq","version":"1.7.1","url":"https://github.com/jqlang/jq","command":"metadata","variant":"custom"},
{"name":"signed","version":"1.2.3","extra":true,"packslip":{"project":"github.com/example/tool"}}
]}`},
		{configFormatTOML, `project="test"
extra=true
[[bins]]
name="go-tool"
go_package="example.com/tool"
command={custom=true}
variant=["custom"]
[[bins]]
name="jq"
version="1.7.1"
url="https://github.com/jqlang/jq"
command="metadata"
variant="custom"
[[bins]]
name="signed"
version="1.2.3"
extra=true
[bins.packslip]
project="github.com/example/tool"
[extension]
custom=true
`},
	} {
		t.Run(string(tt.format), func(t *testing.T) {
			cfg, err := unmarshalConfig(tt.format, []byte(tt.data))
			assert.NilError(t, err)
			applyLibraryDefaults(cfg)
			for _, b := range cfg.Bins {
				assert.NilError(t, b.validatePackslip())
			}
			assert.Assert(t, cfg.Bins[0].Packslip == nil)
			assert.Assert(t, cfg.Bins[0].isLatest())
			assert.Assert(t, cfg.Bins[1].Packslip == nil)
			assert.Equal(t, cfg.Bins[1].AssetPattern, "{name}-{goos}-{goarch}")
			assert.Equal(t, cfg.Bins[2].Packslip.Project, "github.com/example/tool")
			assert.Equal(t, cfg.Bins[2].AssetPattern, "")
		})
	}
}

func TestVerifiedDownload(t *testing.T) {
	content := []byte("executable content")
	size := int64(len(content))
	h256, h512 := sha256.Sum256(content), sha512.Sum512(content)
	release := packslip.Resolved{Artifact: packslip.Artifact{Size: &size}, Digests: map[string]string{"sha256": hex.EncodeToString(h256[:]), "sha512": hex.EncodeToString(h512[:])}}
	var dst bytes.Buffer
	assert.NilError(t, downloadVerified(&dst, bytes.NewReader(content), &release))
	assert.DeepEqual(t, dst.Bytes(), content)
	assert.ErrorContains(t, downloadVerified(&bytes.Buffer{}, strings.NewReader("short"), &release), "size mismatch")
	assert.ErrorContains(t, downloadVerified(&bytes.Buffer{}, bytes.NewReader(append(content, 'x')), &release), "size mismatch")
	modified := bytes.Clone(content)
	modified[0] ^= 1
	assert.ErrorContains(t, downloadVerified(&bytes.Buffer{}, bytes.NewReader(modified), &release), "SHA-256 mismatch")
	release.Digests["sha512"] = strings.Repeat("0", 128)
	assert.ErrorContains(t, downloadVerified(&bytes.Buffer{}, bytes.NewReader(content), &release), "SHA-512 mismatch")
}

func TestPackslipExactExtraction(t *testing.T) {
	for _, tc := range []struct{ format, prefix string }{
		{"tar", "./"},
		{"tar.gz", "././"},
		{"zip", ""},
		{"raw", ""},
	} {
		t.Run(tc.format, func(t *testing.T) {
			var buf bytes.Buffer
			switch tc.format {
			case "tar", "tar.gz":
				var tw *tar.Writer
				var gz *gzip.Writer
				if tc.format == "tar.gz" {
					gz = gzip.NewWriter(&buf)
					tw = tar.NewWriter(gz)
				} else {
					tw = tar.NewWriter(&buf)
				}
				for _, name := range []string{"aaa-wrong-tool", "nested/bin/tool"} {
					assert.NilError(t, tw.WriteHeader(&tar.Header{Name: tc.prefix + name, Mode: 0o755, Size: int64(len(name))}))
					_, err := tw.Write([]byte(name))
					assert.NilError(t, err)
				}
				assert.NilError(t, tw.Close())
				if gz != nil {
					assert.NilError(t, gz.Close())
				}
			case "zip":
				zw := zip.NewWriter(&buf)
				for _, name := range []string{"aaa-wrong-tool", "nested/bin/tool"} {
					w, err := zw.Create(name)
					assert.NilError(t, err)
					_, err = w.Write([]byte(name))
					assert.NilError(t, err)
				}
				assert.NilError(t, zw.Close())
			case "raw":
				buf.WriteString("nested/bin/tool")
			}
			file, err := os.CreateTemp(t.TempDir(), "download-*")
			assert.NilError(t, err)
			defer file.Close()
			_, err = file.Write(buf.Bytes())
			assert.NilError(t, err)
			_, err = file.Seek(0, 0)
			assert.NilError(t, err)
			dest := filepath.Join(t.TempDir(), "local-alias")
			assert.NilError(t, os.WriteFile(dest, nil, 0o600))
			release := &packslip.Resolved{Artifact: packslip.Artifact{Format: tc.format}, Executable: packslip.Executable{Path: "nested/bin/tool", Name: "tool"}}
			assert.NilError(t, extractPackslip(t.Context(), file, dest, release))
			data, err := os.ReadFile(dest)
			assert.NilError(t, err)
			assert.Equal(t, string(data), "nested/bin/tool")
			if tc.format != "raw" {
				_, err = file.Seek(0, 0)
				assert.NilError(t, err)
				release.Executable.Path = "missing"
				assert.ErrorContains(t, extractPackslip(t.Context(), file, dest, release), "missing from archive")
			}
		})
	}
}

func TestPackslipRejectsUnsafeExecutables(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "duplicate", "prefixed duplicate", "parent path"} {
		t.Run(kind, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "archive-*")
			assert.NilError(t, err)
			defer f.Close()
			tw := tar.NewWriter(f)
			h := &tar.Header{Name: "tool", Mode: 0o755}
			wantErr := "duplicate Packslip executable path"
			switch kind {
			case "symlink":
				h.Name = "./tool"
				h.Typeflag = tar.TypeSymlink
				h.Linkname = "other"
				wantErr = "must be a regular file"
			case "hardlink":
				h.Name = "././tool"
				h.Typeflag = tar.TypeLink
				h.Linkname = "other"
				wantErr = "must be a regular file"
			case "parent path":
				h.Name = "./nested/../tool"
				wantErr = "missing from archive"
			}
			assert.NilError(t, tw.WriteHeader(h))
			if kind == "duplicate" || kind == "prefixed duplicate" {
				if kind == "prefixed duplicate" {
					h.Name = "./tool"
				}
				assert.NilError(t, tw.WriteHeader(h))
			}
			assert.NilError(t, tw.Close())
			_, err = f.Seek(0, 0)
			assert.NilError(t, err)
			dest := filepath.Join(t.TempDir(), "out")
			assert.NilError(t, os.WriteFile(dest, nil, 0o600))
			r := &packslip.Resolved{Artifact: packslip.Artifact{Format: "tar"}, Executable: packslip.Executable{Path: "tool"}}
			assert.ErrorContains(t, extractPackslip(t.Context(), f, dest, r), wantErr)
		})
	}
}

func TestPackslipReceiptsBindSourceAndSelection(t *testing.T) {
	f := newInstallationFixture(t, "packslip")
	b, item := f.b, f.b.config.Bins[0]
	path, err := b.Get(t.Context(), item.Name)
	assert.NilError(t, err)
	var marker versionMarkerDocument
	assert.NilError(t, json.Unmarshal(installationRead(t, filepath.Join(b.VersionsDir, item.Name, item.Version)), &marker))
	receipt := marker.Packslip
	assert.Assert(t, receipt != nil)
	// An old marker without a receipt requires a verified reinstall.
	assert.NilError(t, b.markVersion(item, installResult{}))
	_, err = b.Get(t.Context(), item.Name)
	assert.NilError(t, err)
	assert.Equal(t, f.downloads["tool1@1.0.0"], 2)
	for _, change := range []func(*bin){func(b *bin) { b.Packslip.Command = "other" }, func(b *bin) { b.Packslip.Variant = "other" }, func(b *bin) { b.Packslip.Project = "github.com/other/tool" }, func(b *bin) { b.Packslip = nil; b.source = &goSource{} }} {
		changed := *item
		selection := *item.Packslip
		changed.Packslip = &selection
		change(&changed)
		ok, err := b.installed(t.Context(), &changed)
		assert.NilError(t, err)
		assert.Assert(t, !ok)
	}
	assert.NilError(t, os.WriteFile(path, []byte("changed executable"), 0o755))
	_, err = b.Get(t.Context(), item.Name)
	assert.NilError(t, err)
	assert.Equal(t, string(installationRead(t, path)), "tool1@1.0.0")
	assert.Equal(t, f.downloads["tool1@1.0.0"], 3)
	_, err = b.Get(t.Context(), item.Name)
	assert.NilError(t, err)
	assert.Equal(t, f.downloads["tool1@1.0.0"], 3, "repaired receipt permits cache reuse")
	// Removing the binary cache receipt never removes remembered trust.
	assert.NilError(t, b.removeVersionMarker(item))
	r := &packslip.Resolver{StateDir: f.stateDir}
	ok, err := r.Accepted(item.Packslip.Project, &receipt.Release)
	assert.NilError(t, err)
	assert.Assert(t, ok)
	assert.NilError(t, b.markVersion(item, installResult{Packslip: receipt, ExpectedSHA256: receipt.ExecutableSHA256}))
	assert.NilError(t, os.Remove(filepath.Join(f.stateDir, "trust.json")))
	ok, err = b.installed(t.Context(), item)
	assert.NilError(t, err)
	assert.Assert(t, !ok)
	assert.NilError(t, os.WriteFile(path, []byte("replaced by another installer"), 0o755))
	assert.ErrorContains(t, b.markVersion(item, installResult{Packslip: receipt, ExpectedSHA256: receipt.ExecutableSHA256}), "changed before its version marker")
	f.assertClean(t)
}

func TestRequirementVersions(t *testing.T) {
	for _, tt := range []struct {
		actual, min string
		want        int
		known       bool
	}{
		{"2.10", "2.9", 1, true}, {"17", "17.0.0", 0, true}, {"2.31", "2.34", -1, true}, {"unknown", "1", 0, false}, {"1.2-rc1", "1", 0, false},
	} {
		comparison, known := compareRequirementVersion(tt.actual, tt.min)
		assert.Equal(t, comparison, tt.want)
		assert.Equal(t, known, tt.known)
	}
}

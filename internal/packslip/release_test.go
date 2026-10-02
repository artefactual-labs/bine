package packslip

import (
	"encoding/json"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func sampleStatement() Statement {
	var s Statement
	_ = json.Unmarshal([]byte(`{
	  "_type":"https://in-toto.io/Statement/v1", "predicateType":"https://packslip.dev/release/v1",
	  "subject":[{"name":"tool.tar.gz","digest":{"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}],
	  "predicate":{"project":"github.com/example/tool","version":"1.2.3+build.1","published_at":"2026-09-01T00:00:00Z",
	  "artifacts":[{"name":"tool.tar.gz","size":42,"format":"tar.gz","bin":["pkg/bin/tool"]}],
	  "identity":{"scheme":"sigstore-oidc","issuer":"https://token.actions.githubusercontent.com","key_id":"https://github.com/example/tool/.github/workflows/release.yml@refs/tags/v1.2.3"}}
	}`), &s)
	return s
}

func TestParseRelease(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*Statement)
		want   string
	}{
		{"valid", func(*Statement) {}, ""},
		{"wrong predicate", func(s *Statement) { s.PredicateType = "other" }, "release/v1"},
		{"partial version", func(s *Statement) { s.Predicate.Version = "1.2" }, "semantic version"},
		{"missing size", func(s *Statement) { s.Predicate.Artifacts[0].Size = nil }, "invalid or duplicate artifact"},
		{"bad digest", func(s *Statement) { s.Subject[0].Digest["sha256"] = "abc" }, "subject"},
		{"duplicate subject", func(s *Statement) { s.Subject = append(s.Subject, s.Subject[0]) }, "subject"},
		{"duplicate platform", func(s *Statement) {
			a := s.Predicate.Artifacts[0]
			a.Name = "other.tar.gz"
			s.Predicate.Artifacts = append(s.Predicate.Artifacts, a)
			sub := s.Subject[0]
			sub.Name = a.Name
			s.Subject = append(s.Subject, sub)
		}, "duplicate artifact platform"},
		{"unsafe path", func(s *Statement) { s.Predicate.Artifacts[0].Bin[0].Path = "../tool" }, "executable"},
		{"unsafe command", func(s *Statement) { s.Predicate.Artifacts[0].Bin[0].Name = "../tool" }, "executable"},
		{"HTTP URL", func(s *Statement) { s.Predicate.Artifacts[0].URL = "http://example.com/tool" }, "HTTPS"},
		{"repackager", func(s *Statement) { s.Predicate.AttestedBy = "repackager" }, "vendor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := sampleStatement()
			tt.change(&s)
			data, err := json.Marshal(s)
			assert.NilError(t, err)
			parsed, err := Parse(data)
			if tt.want != "" {
				assert.ErrorContains(t, err, tt.want)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, parsed.Predicate.Version, "1.2.3+build.1")
		})
	}
	_, err := Parse([]byte(`{"_type":"a","_type":"b"}`))
	assert.ErrorContains(t, err, "duplicate")
}

func TestSelectArtifactAndCommand(t *testing.T) {
	s := sampleStatement()
	portable := s.Predicate.Artifacts[0]
	linux := portable
	linux.Name = "linux.tar.gz"
	linux.OS = "linux"
	linux.Arch = "x86_64"
	musl := linux
	musl.Name = "musl.tar.gz"
	musl.Libc = "musl"
	variant := linux
	variant.Name = "baseline.tar.gz"
	variant.Variant = "baseline"
	zip := linux
	zip.Name = "linux.zip"
	zip.Format = "zip"
	s.Predicate.Artifacts = []Artifact{portable, zip, variant, musl, linux}
	for _, tt := range []struct {
		host          Host
		variant, want string
	}{
		{Host{"darwin", "aarch64", ""}, "", "tool.tar.gz"},
		{Host{"linux", "x86_64", "gnu"}, "", "linux.tar.gz"},
		{Host{"linux", "x86_64", "musl"}, "", "musl.tar.gz"},
		{Host{"linux", "x86_64", ""}, "baseline", "baseline.tar.gz"},
	} {
		a, e, err := s.Select(tt.host, tt.variant, "", "local-alias")
		assert.NilError(t, err)
		assert.Equal(t, a.Name, tt.want)
		assert.Equal(t, e.Path, "pkg/bin/tool")
	}
	_, _, err := s.Select(Host{"linux", "x86_64", ""}, "missing", "", "")
	assert.ErrorContains(t, err, "found 0")
	s.Predicate.Artifacts = []Artifact{linux, linux}
	_, _, err = s.Select(Host{"linux", "x86_64", ""}, "", "", "")
	assert.ErrorContains(t, err, "found 2")
	linux.Bin = []Executable{{Path: "bin/foo", Name: "foo"}, {Path: "bin/bar", Name: "bar"}}
	s.Predicate.Artifacts = []Artifact{linux}
	_, _, err = s.Select(Host{"linux", "x86_64", ""}, "", "", "alias")
	assert.ErrorContains(t, err, "foo, bar")
	_, e, err := s.Select(Host{"linux", "x86_64", ""}, "", "bar", "alias")
	assert.NilError(t, err)
	assert.Equal(t, e.Path, "bin/bar")
	_, _, err = s.Select(Host{"linux", "x86_64", ""}, "", "missing", "foo")
	assert.ErrorContains(t, err, "select command")
}

func TestProjectAndTagVersions(t *testing.T) {
	for _, p := range []string{"https://github.com/a/b", "github.com/a/b/../x", "github.com/a/b/", "github.com/a", "github.com/a/b?x"} {
		_, _, err := Project(p)
		assert.Assert(t, err != nil, p)
	}
	for tag, want := range map[string]string{"v1.2.3": "1.2.3", "cli/v1.2.3": "1.2.3", "tool-1.2.3": "1.2.3", "cli_v1.2.3": "1.2.3", "v1.2.3+build.1": "1.2.3+build.1", "v1.2": "1.2.0", "v25.07.1": "25.7.1", "2026-08-31": "2026.8.31", "foo-1.2.3": ""} {
		assert.Equal(t, tagVersion(tag, "example/tool", "cli"), want, tag)
	}
	assert.Assert(t, !ValidVersion(strings.Repeat("1", 10)))
}

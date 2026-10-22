package packslip

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestUpstreamStatementValidity(t *testing.T) {
	data, err := os.ReadFile("testdata/statement-validity.json")
	assert.NilError(t, err)
	var vectors struct {
		Cases []struct {
			Name, Expect string
			Statement    json.RawMessage
		}
	}
	assert.NilError(t, json.Unmarshal(data, &vectors))
	assert.Assert(t, len(vectors.Cases) > 0)
	for _, tc := range vectors.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := Parse(tc.Statement)
			switch tc.Expect {
			case "accept":
				assert.NilError(t, err)
			case "reject":
				assert.Assert(t, err != nil, "accepted invalid upstream statement")
			default:
				t.Fatalf("unknown upstream expectation %q", tc.Expect)
			}
		})
	}
}

func TestParseRequirements(t *testing.T) {
	for _, tc := range []struct{ name, requirements, want string }{
		{"valid", `{"libs":["libc.so.6"],"bin":[{"name":"git","min":"2.43.0-rc1+build"}]}`, ""},
		{"empty", `{}`, ""},
		{"duplicate library", `{"libs":["libc.so.6","libc.so.6"]}`, "required library"},
		{"library path", `{"libs":["../libc.so"]}`, "required library"},
		{"library whitespace", `{"libs":["lib c.so"]}`, "required library"},
		{"empty library", `{"libs":[""]}`, "required library"},
		{"duplicate command", `{"bin":[{"name":"git"},{"name":"git"}]}`, "required command"},
		{"command whitespace", `{"bin":[{"name":"git status"}]}`, "required command"},
		{"command suffix", `{"bin":[{"name":"git.EXE"}]}`, "required command"},
		{"command path", `{"bin":[{"name":"../git"}]}`, "required command"},
		{"minimum prefix", `{"bin":[{"name":"git","min":"v2.0"}]}`, "minimum version"},
		{"minimum whitespace", `{"bin":[{"name":"git","min":"2.0 --help"}]}`, "minimum version"},
		{"own command", `{"bin":[{"name":"tool"}]}`, "provided by this release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sampleStatement()
			assert.NilError(t, json.Unmarshal([]byte(tc.requirements), &s.Predicate.Artifacts[0].Requires))
			data, err := json.Marshal(s)
			assert.NilError(t, err)
			_, err = Parse(data)
			if tc.want == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.want)
			}
		})
	}
}

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
		{"domain project policy", func(s *Statement) { s.Predicate.Project = "example.com/tool" }, "github.com/owner/repo"},
		{"key signing policy", func(s *Statement) { s.Predicate.Identity.Scheme = "sigstore-key" }, "vendor"},
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

func TestParseRequiresCommandInLaterArtifact(t *testing.T) {
	s := sampleStatement()
	a := s.Predicate.Artifacts[0]
	a.Name, a.OS = "helper.tar.gz", "linux"
	a.Bin = []Executable{{Path: "helper", Name: "helper"}}
	s.Predicate.Artifacts = append(s.Predicate.Artifacts, a)
	s.Subject = append(s.Subject, Subject{Name: a.Name, Digest: s.Subject[0].Digest})
	assert.NilError(t, json.Unmarshal([]byte(`{"bin":[{"name":"helper"}]}`), &s.Predicate.Artifacts[0].Requires))
	data, err := json.Marshal(s)
	assert.NilError(t, err)
	_, err = Parse(data)
	assert.ErrorContains(t, err, "provided by this release")
}

func TestParseResourceDeclarations(t *testing.T) {
	for _, tc := range []struct{ name, resource, want string }{
		{"unknown command", `{"kind":"icon","exec":["other"]}`, "not provided"},
		{"environment without command", `{"kind":"icon","archive":"icon.svg","env":{"X":"y"}}`, "requires exec"},
		{"invalid environment", `{"kind":"icon","exec":["tool"],"env":{"A=B":"y"}}`, "environment name"},
		{"missing shell", `{"kind":"completion","archive":"tool.bash"}`, "exactly one"},
		{"both shell forms", `{"kind":"completion","shell":"bash","shells":["bash"],"archive":"tool.bash"}`, "exactly one"},
		{"empty shell", `{"kind":"completion","shells":[""],"exec":["tool","{shell}"]}`, "invalid completion shell"},
		{"static multiple shells", `{"kind":"completion","shells":["bash"],"archive":"tool.bash"}`, "static completion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sampleStatement()
			assert.NilError(t, json.Unmarshal([]byte("["+tc.resource+"]"), &s.Predicate.Resources))
			data, err := json.Marshal(s)
			assert.NilError(t, err)
			_, err = Parse(data)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseWindowsCommandReferences(t *testing.T) {
	for _, tc := range []struct{ name, executable, reference string }{
		{"extension in reference", "tool", "tool.exe"},
		{"mixed case reference", "tool", "tool.ExE"},
		{"legacy executable name", "tool.exe", "tool"},
		{"mixed case executable name", "tool.ExE", "tool"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sampleStatement()
			s.Predicate.Artifacts[0].Bin = []Executable{{Path: "bin/tool.exe", Name: tc.executable}}
			assert.NilError(t, json.Unmarshal([]byte(`[{"kind":"completion","shells":["powershell"],"exec":["`+tc.reference+`","{shell}"]}]`), &s.Predicate.Resources))
			data, err := json.Marshal(s)
			assert.NilError(t, err)
			parsed, err := Parse(data)
			assert.NilError(t, err)
			assert.DeepEqual(t, parsed.Predicate.Artifacts[0].Bin, s.Predicate.Artifacts[0].Bin)

			assert.NilError(t, json.Unmarshal([]byte(`{"bin":[{"name":"tool"}]}`), &s.Predicate.Artifacts[0].Requires))
			data, err = json.Marshal(s)
			assert.NilError(t, err)
			_, err = Parse(data)
			assert.ErrorContains(t, err, "provided by this release")
		})
	}
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

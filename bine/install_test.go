package bine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHelperProcessWithSuccess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := strings.Join(os.Args, " ")
	gobin := ""
	for _, item := range os.Environ() {
		if value, ok := strings.CutPrefix(item, "GOBIN="); ok {
			gobin = value
			break
		}
	}
	if gobin == "" {
		os.Exit(1)
	}

	fields := strings.Fields(args)
	pkgWithVersion := fields[len(fields)-1]
	pkg, _, ok := strings.Cut(pkgWithVersion, "@")
	if !ok {
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(gobin, defaultGoBinaryName(pkg)), []byte("binary"), 0o755); err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

func TestHelperProcessWithError(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	os.Exit(1)
}

func TestGoInstall(t *testing.T) {
	t.Run("Fails if go binary is not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		b := &bin{}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)

		assert.ErrorContains(t, err, "cannot find 'go' command")
	})

	t.Run("Fails if go binary returns an error", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithError")
		b := &bin{
			GoPackage: "github.com/artefactual-labs/bine",
			Version:   "v0.1.0",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.Error(t, err, "`go install github.com/artefactual-labs/bine@v0.1.0` failed: exit status 1\nstderr: (no stderr output)")
	})

	t.Run("Uses `go install`", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithSuccess")
		b := &bin{
			Name:      "bine",
			GoPackage: "github.com/artefactual-labs/bine",
			Version:   "v0.1.0",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)
		_, statErr := os.Stat(filepath.Join(binDir, "bine"))
		assert.NilError(t, statErr)
	})

	t.Run("Uses `go install` with @latest", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithSuccess")
		b := &bin{
			Name:      "bine",
			GoPackage: "github.com/artefactual-labs/bine",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)
		_, statErr := os.Stat(filepath.Join(binDir, "bine"))
		assert.NilError(t, statErr)
	})

	t.Run("Uses configured binary name", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithSuccess")
		b := &bin{
			Name:      "custom-bine",
			GoPackage: "github.com/artefactual-labs/bine",
			Version:   "v0.1.0",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)
		_, statErr := os.Stat(filepath.Join(binDir, "custom-bine"))
		assert.NilError(t, statErr)
	})

	t.Run("Uses versioned module root package name", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithSuccess")
		b := &bin{
			Name:      "yq-bin",
			GoPackage: "github.com/mikefarah/yq/v4",
			Version:   "v4.40.5",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)
		_, statErr := os.Stat(filepath.Join(binDir, "yq-bin"))
		assert.NilError(t, statErr)
	})

	t.Run("Replaces existing binary", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithSuccess")
		b := &bin{
			Name:      "bine",
			GoPackage: "github.com/artefactual-labs/bine",
			Version:   "v0.1.0",
		}
		binDir := t.TempDir()
		targetPath := filepath.Join(binDir, "bine")
		err := os.WriteFile(targetPath, []byte("old-binary"), 0o755)
		assert.NilError(t, err)

		err = goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)

		blob, err := os.ReadFile(targetPath)
		assert.NilError(t, err)
		assert.Equal(t, string(blob), "binary")
	})
}

func TestDefaultGoBinaryName(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
		want string
	}{
		{
			name: "plain package",
			pkg:  "github.com/artefactual-labs/bine",
			want: "bine",
		},
		{
			name: "major version suffix at end",
			pkg:  "github.com/mikefarah/yq/v4",
			want: "yq",
		},
		{
			name: "major version suffix before command path",
			pkg:  "github.com/cli/cli/v2/cmd/gh",
			want: "gh",
		},
		{
			name: "module root with major version suffix",
			pkg:  "github.com/cli/cli/v2",
			want: "cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, defaultGoBinaryName(tt.pkg), tt.want)
		})
	}
}

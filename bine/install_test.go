package bine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// TestHelperProcessGoVersionM handles the "go version -m <binary>" command in tests.
func TestHelperProcessGoVersionM(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	for i, a := range args {
		if a == "version" && i+1 < len(args) && args[i+1] == "-m" {
			binaryPath := args[i+2]
			// Simulate "go version -m" output format:
			//   <path>: go1.21.0
			//           path    github.com/foo/bar/cmd/tool
			//           mod     github.com/foo/bar      v1.2.3  h1:abc
			fmt.Printf("%s: go1.21.0\n", binaryPath)
			fmt.Printf("\tpath\tgithub.com/foo/bar/cmd/tool\n")
			fmt.Printf("\tmod\tgithub.com/foo/bar\tv1.2.3\th1:abc\n")
			os.Exit(0)
		}
	}

	os.Exit(1)
}

func TestHelperProcessInstallButFailGoVersionM(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	for i, a := range args {
		if a == "version" && i+1 < len(args) && args[i+1] == "-m" {
			os.Exit(1)
		}
	}

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

	fields := strings.Fields(strings.Join(os.Args, " "))
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

func TestHelperProcessWithCounter(t *testing.T) {
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

	counterPath := os.Getenv("BINE_HELPER_COUNTER")
	if counterPath == "" {
		os.Exit(1)
	}

	count := 0
	if blob, err := os.ReadFile(counterPath); err == nil {
		count, err = strconv.Atoi(strings.TrimSpace(string(blob)))
		if err != nil {
			os.Exit(1)
		}
	} else if !os.IsNotExist(err) {
		os.Exit(1)
	}
	count++

	if err := os.WriteFile(counterPath, []byte(strconv.Itoa(count)), 0o644); err != nil {
		os.Exit(1)
	}

	fields := strings.Fields(args)
	pkgWithVersion := fields[len(fields)-1]
	pkg, _, ok := strings.Cut(pkgWithVersion, "@")
	if !ok {
		os.Exit(1)
	}

	content := fmt.Sprintf("binary-%d", count)
	if err := os.WriteFile(filepath.Join(gobin, defaultGoBinaryName(pkg)), []byte(content), 0o755); err != nil {
		os.Exit(1)
	}

	os.Exit(0)
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

func TestGoInstalledVersion(t *testing.T) {
	t.Run("Returns version from go version -m output", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessGoVersionM")

		version, err := goInstalledVersion(t.Context(), "/fake/binary")
		assert.NilError(t, err)
		assert.Equal(t, version, "1.2.3")
	})

	t.Run("Returns error when go binary not found", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		_, err := goInstalledVersion(t.Context(), "/fake/binary")
		assert.ErrorContains(t, err, "cannot find 'go' command")
	})

	t.Run("Returns error when command fails", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithError")

		_, err := goInstalledVersion(t.Context(), "/fake/binary")
		assert.ErrorContains(t, err, "go version -m")
	})
}

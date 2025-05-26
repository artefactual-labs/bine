package bine

import (
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHelperProcessWithSuccess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := strings.Join(os.Args, " ")
	if !strings.HasSuffix(args, "go install github.com/artefactual-labs/bine@v0.1.0") &&
		!strings.HasSuffix(args, "go install github.com/artefactual-labs/bine@latest") {
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
			GoPackage: "github.com/artefactual-labs/bine",
			Version:   "v0.1.0",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)
	})

	t.Run("Uses `go install` with @latest", func(t *testing.T) {
		injectFakeExec(t, "TestHelperProcessWithSuccess")
		b := &bin{
			GoPackage: "github.com/artefactual-labs/bine",
		}
		binDir := t.TempDir()

		err := goInstall(t.Context(), b, binDir)
		assert.NilError(t, err)
	})
}

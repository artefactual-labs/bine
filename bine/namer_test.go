package bine

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHelperRustcTriple(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := strings.Join(os.Args, " ")
	if strings.Contains(args, "uname -s") {
		fmt.Println("Linux")
	} else if strings.Contains(args, "uname -m") {
		fmt.Println("x86_64")
	} else {
		fmt.Println("x86_64-unknown-linux-gnu")
	}

	os.Exit(0)
}

func TestHelperRustcFailed(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := strings.Join(os.Args, " ")
	if strings.Contains(args, "uname -s") {
		fmt.Println("Linux")
		os.Exit(0)
	} else if strings.Contains(args, "uname -m") {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

func TestNamer(t *testing.T) {
	t.Run("Computes triple with rustc", func(t *testing.T) {
		injectFakeExec(t, "TestHelperRustcTriple")
		n, err := createNamer()
		assert.NilError(t, err)

		bins := []*bin{{AssetPattern: "{triple}"}}
		n.run(bins)

		t.Log(bins[0].asset)
	})

	t.Run("Computes triple without rustc", func(t *testing.T) {
		injectFakeExec(t, "TestHelperRustcFailed")
		n, err := createNamer()
		assert.NilError(t, err)

		bins := []*bin{{AssetPattern: "{triple}"}}
		n.run(bins)

		t.Log(bins[0].asset)
	})
}

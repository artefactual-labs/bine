package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"bine": main,
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			// Enables testing of `bine version` command.
			env.Setenv("GOVERSION", runtime.Version())
			// Set up environment variables for general testing.
			env.Setenv("HOME", filepath.Join(env.Getenv("TMPDIR"), "homedir"))
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"setup": func(ts *testscript.TestScript, neg bool, args []string) {
				// Remove the cache directory if it exists.
				err := os.RemoveAll(filepath.Join(ts.Getenv("HOME"), ".cache"))
				ts.Check(err)
				// Populate the configuration file.
				if len(args) > 0 {
					workDir := ts.Getenv("WORK")
					filename := args[0]
					config := ts.ReadFile(filename)
					err = os.WriteFile(filepath.Join(workDir, ".bine.json"), []byte(config), 0o644)
					ts.Check(err)
				}
			},
		},
	})
}

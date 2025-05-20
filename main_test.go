package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var ghToken = os.Getenv("BINE_GITHUB_API_TOKEN")

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
			// Pass the GitHub API token to the test environment.
			env.Setenv("BINE_GITHUB_API_TOKEN", ghToken)
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// `setup` flushes the cache directory entirely and populates the configuration file.
			"setup": func(ts *testscript.TestScript, neg bool, args []string) {
				cacheDir := filepath.Join(ts.Getenv("HOME"), ".cache")
				projectDir := filepath.Join(ts.Getenv("WORK"), "project")
				// Remove the cache directory if it exists.
				ts.Check(os.RemoveAll(cacheDir))
				ts.Check(os.RemoveAll(projectDir))
				// Create the project directory.
				ts.Check(os.Mkdir(projectDir, os.FileMode(0o750)))
				ts.Check(ts.Chdir(projectDir))
				// Populate the configuration file.
				if len(args) > 0 {
					config := ts.ReadFile(filepath.Join(ts.Getenv("WORK"), args[0]))
					ts.Check(os.WriteFile(filepath.Join(projectDir, ".bine.json"), []byte(config), 0o644))
				}
			},
			// `config` only rewrites configuration file.
			"config": func(ts *testscript.TestScript, neg bool, args []string) {
				projectDir := filepath.Join(ts.Getenv("WORK"), "project")
				config := ts.ReadFile(filepath.Join(ts.Getenv("WORK"), args[0]))
				ts.Check(os.WriteFile(filepath.Join(projectDir, ".bine.json"), []byte(config), 0o644))
			},
		},
	})
}

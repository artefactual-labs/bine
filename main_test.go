package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/peterbourgon/ff/v4"
	"github.com/rogpeppe/go-internal/testscript"
	"golang.org/x/oauth2"
	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/cmd/rootcmd"
	"github.com/artefactual-labs/bine/internal/auth"
)

var ghToken = os.Getenv("BINE_GITHUB_API_TOKEN")

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"bine":           main,
		"bine-auth-test": authTestMain,
	})
}

func authTestMain() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			fmt.Fprint(w, `{
				"device_code":"device-code",
				"user_code":"ABCD-EFGH",
				"verification_uri":"https://example.com/device",
				"expires_in":60,
				"interval":1
			}`)
		case "/token":
			fmt.Fprint(w, `{
				"access_token":"access-token",
				"token_type":"bearer",
				"expires_in":3600
			}`)
		case "/user":
			fmt.Fprint(w, `{"login":"octocat"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := auth.Provider{
		Name:     "GitHub",
		Host:     "github.com",
		ClientID: "client-id",
		Endpoint: oauth2.Endpoint{
			DeviceAuthURL: server.URL + "/device",
			TokenURL:      server.URL + "/token",
			AuthStyle:     oauth2.AuthStyleInParams,
		},
		IdentityURL:  server.URL + "/user",
		AccountField: "login",
	}
	manager := auth.NewManager(
		testCredentialStore{credential: auth.Credential{
			Account:     "octocat",
			AccessToken: "access-token",
			Expiry:      time.Now().Add(time.Hour),
		}},
		auth.NewRegistry(provider),
		server.Client(),
	)

	err := execWithAuthManager(
		context.Background(),
		os.Args[1:],
		os.Stdin,
		os.Stdout,
		os.Stderr,
		manager,
	)
	if err != nil && !errors.Is(err, ff.ErrHelp) {
		fmt.Fprintf(os.Stderr, "Command failed: %v.\n", err)
		os.Exit(1)
	}
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
			// Pass the cache directory to the test environment.
			env.Setenv("BINE_CACHE_DIR", filepath.Join(env.Getenv("TMPDIR"), "homedir", ".cache"))
			// These are useful during assertions.
			env.Setenv("GOOS", runtime.GOOS)
			env.Setenv("GOARCH", runtime.GOARCH)
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
					configPath := filepath.Join(ts.Getenv("WORK"), args[0])
					config := ts.ReadFile(configPath)
					ts.Check(os.WriteFile(filepath.Join(projectDir, targetConfigFilename(configPath)), []byte(config), 0o644))
				}
			},
			// `config` only rewrites configuration file.
			"config": func(ts *testscript.TestScript, neg bool, args []string) {
				projectDir := filepath.Join(ts.Getenv("WORK"), "project")
				configPath := filepath.Join(ts.Getenv("WORK"), args[0])
				config := ts.ReadFile(configPath)
				ts.Check(os.WriteFile(filepath.Join(projectDir, targetConfigFilename(configPath)), []byte(config), 0o644))
			},
		},
	})
}

func targetConfigFilename(path string) string {
	if filepath.Ext(path) == ".toml" {
		return ".bine.toml"
	}
	return ".bine.json"
}

type testCredentialStore struct {
	credential auth.Credential
	err        error
}

func (s testCredentialStore) Get(string) (auth.Credential, error) {
	return s.credential, s.err
}

func (testCredentialStore) Set(string, auth.Credential) error {
	return nil
}

func (testCredentialStore) Delete(string) error {
	return nil
}

func TestResolveGitHubAPIToken(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		store    testCredentialStore
		want     string
	}{
		{
			name:     "explicit token takes precedence",
			explicit: "explicit-token",
			store:    testCredentialStore{credential: auth.Credential{AccessToken: "stored-token"}},
			want:     "explicit-token",
		},
		{
			name:  "stored token",
			store: testCredentialStore{credential: auth.Credential{AccessToken: "stored-token"}},
			want:  "stored-token",
		},
		{
			name:  "missing token uses anonymous access",
			store: testCredentialStore{err: auth.ErrCredentialNotFound},
		},
		{
			name:  "unavailable store uses anonymous access",
			store: testCredentialStore{err: auth.ErrCredentialStoreUnavailable},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := &rootcmd.RootConfig{
				GitHubAPIToken: test.explicit,
				Auth: auth.NewManager(
					test.store,
					auth.DefaultRegistry(),
					nil,
				),
			}

			got, err := resolveGitHubAPIToken(context.Background(), root)
			assert.NilError(t, err)
			assert.Equal(t, got, test.want)
		})
	}
}

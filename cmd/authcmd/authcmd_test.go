package authcmd

import (
	"bytes"
	"context"
	"testing"

	"golang.org/x/oauth2"
	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/cmd/rootcmd"
	"github.com/artefactual-labs/bine/internal/auth"
)

type fakeManager struct {
	credential auth.Credential
	loginHost  string
	logoutHost string
	statusErr  error
	logoutErr  error
}

func (m *fakeManager) Providers() []auth.Provider {
	return []auth.Provider{{Name: "GitHub", Host: "github.com"}}
}

func (m *fakeManager) BeginDeviceAuth(_ context.Context, host string) (auth.Provider, *oauth2.DeviceAuthResponse, error) {
	m.loginHost = host
	return auth.Provider{Name: "GitHub", Host: host}, &oauth2.DeviceAuthResponse{
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://github.com/login/device",
	}, nil
}

func (m *fakeManager) CompleteDeviceAuth(context.Context, auth.Provider, *oauth2.DeviceAuthResponse) (auth.Credential, error) {
	return m.credential, nil
}

func (m *fakeManager) Status(context.Context, string) (auth.Credential, error) {
	return m.credential, m.statusErr
}

func (m *fakeManager) Logout(host string) error {
	m.logoutHost = host
	return m.logoutErr
}

func TestLogin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := rootcmd.New(nil, &stdout, &stderr)
	cfg := New(root)
	manager := &fakeManager{credential: auth.Credential{Account: "octocat"}}
	cfg.manager = manager
	cfg.noBrowser = true

	err := cfg.ExecLogin(context.Background(), nil)
	assert.NilError(t, err)
	assert.Equal(t, manager.loginHost, "github.com")
	assert.Assert(t, bytes.Contains(stdout.Bytes(), []byte("ABCD-EFGH")))
	assert.Assert(t, bytes.Contains(stdout.Bytes(), []byte("Logged in to github.com as octocat.")))
	assert.Equal(t, stderr.String(), "")
}

func TestStatusWhenLoggedOut(t *testing.T) {
	var stdout bytes.Buffer
	root := rootcmd.New(nil, &stdout, nil)
	cfg := New(root)
	cfg.manager = &fakeManager{statusErr: auth.ErrCredentialNotFound}

	err := cfg.ExecStatus(context.Background(), nil)
	assert.NilError(t, err)
	assert.Equal(t, stdout.String(), "github.com: not logged in\n")
}

func TestLogoutWhenLoggedOut(t *testing.T) {
	var stdout bytes.Buffer
	root := rootcmd.New(nil, &stdout, nil)
	cfg := New(root)
	manager := &fakeManager{logoutErr: auth.ErrCredentialNotFound}
	cfg.manager = manager

	err := cfg.ExecLogout(context.Background(), []string{"github.com"})
	assert.NilError(t, err)
	assert.Equal(t, manager.logoutHost, "github.com")
	assert.Equal(t, stdout.String(), "Not logged in to github.com.\n")
}

func TestHostArgRejectsExtraArguments(t *testing.T) {
	_, err := hostArg([]string{"github.com", "extra"})
	assert.ErrorContains(t, err, "at most one host")
}

func TestGlobalFlagsAfterSubcommand(t *testing.T) {
	tests := []string{"status", "logout"}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			root := rootcmd.New(nil, nil, nil)
			New(root)

			err := root.Command.Parse([]string{"auth", command, "--verbosity=2"})
			assert.NilError(t, err)
			assert.Equal(t, root.Verbosity, 2)
		})
	}
}

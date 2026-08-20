package auth

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()

	provider, err := registry.Lookup("https://GitHub.com/")
	assert.NilError(t, err)
	assert.Equal(t, provider.Host, "github.com")
	assert.Equal(t, provider.ClientID, githubClientID)
	assert.Equal(t, provider.OAuthConfig().Endpoint.DeviceAuthURL, "https://github.com/login/device/code")

	_, err = registry.Lookup("gitlab.com")
	assert.ErrorContains(t, err, `authentication for "gitlab.com" is not supported`)
}

func TestRegistryRejectsHostPaths(t *testing.T) {
	_, err := DefaultRegistry().Lookup("https://github.com/owner/repo")
	assert.ErrorContains(t, err, "invalid authentication host")
}

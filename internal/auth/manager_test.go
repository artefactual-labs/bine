package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"gotest.tools/v3/assert"
)

type memoryStore struct {
	credentials map[string]Credential
}

func newMemoryStore() *memoryStore {
	return &memoryStore{credentials: map[string]Credential{}}
}

func (s *memoryStore) Get(host string) (Credential, error) {
	credential, ok := s.credentials[host]
	if !ok {
		return Credential{}, ErrCredentialNotFound
	}

	return credential, nil
}

func (s *memoryStore) Set(host string, credential Credential) error {
	s.credentials[host] = credential
	return nil
}

func (s *memoryStore) Delete(host string) error {
	if _, ok := s.credentials[host]; !ok {
		return ErrCredentialNotFound
	}
	delete(s.credentials, host)
	return nil
}

func TestManagerBeginDeviceAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Method, http.MethodPost)
		assert.NilError(t, r.ParseForm())
		assert.Equal(t, r.Form.Get("client_id"), "client-id")
		assert.Equal(t, r.Form.Get("scope"), "")
		w.Header().Set("Content-Type", "application/json")
		assert.NilError(t, json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "device-code",
			"user_code":        "ABCD-EFGH",
			"verification_uri": "https://example.com/device",
			"expires_in":       900,
			"interval":         5,
		}))
	}))
	defer server.Close()

	provider := testProvider(server.URL)
	manager := NewManager(newMemoryStore(), NewRegistry(provider), nil)

	gotProvider, response, err := manager.BeginDeviceAuth(context.Background(), provider.Host)
	assert.NilError(t, err)
	assert.Equal(t, gotProvider.Host, provider.Host)
	assert.Equal(t, response.UserCode, "ABCD-EFGH")
	assert.Equal(t, response.VerificationURI, "https://example.com/device")
}

func TestManagerCompletesDeviceAuthAndPersistsCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			assert.Equal(t, r.Method, http.MethodPost)
			assert.NilError(t, r.ParseForm())
			assert.Equal(t, r.Form.Get("client_id"), "client-id")
			assert.Equal(t, r.Form.Get("device_code"), "device-code")
			assert.Equal(t, r.Form.Get("grant_type"), "urn:ietf:params:oauth:grant-type:device_code")
			w.Header().Set("Content-Type", "application/json")
			assert.NilError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":             "access-token",
				"token_type":               "bearer",
				"refresh_token":            "refresh-token",
				"expires_in":               3600,
				"refresh_token_expires_in": 7200,
			}))
		case "/user":
			assert.Equal(t, r.Method, http.MethodGet)
			assert.Equal(t, r.Header.Get("Authorization"), "Bearer access-token")
			w.Header().Set("Content-Type", "application/json")
			assert.NilError(t, json.NewEncoder(w).Encode(map[string]string{"login": "octocat"}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := testProvider(server.URL)
	store := newMemoryStore()
	manager := NewManager(store, NewRegistry(provider), server.Client())
	response := &oauth2.DeviceAuthResponse{
		DeviceCode: "device-code",
		Expiry:     time.Now().Add(time.Minute),
		Interval:   1,
	}

	credential, err := manager.CompleteDeviceAuth(context.Background(), provider, response)
	assert.NilError(t, err)
	assert.Equal(t, credential.Account, "octocat")
	assert.Equal(t, credential.AccessToken, "access-token")
	assert.Equal(t, credential.RefreshToken, "refresh-token")
	assert.Assert(t, credential.Expiry.After(time.Now()))
	assert.Assert(t, credential.RefreshExpiry.After(time.Now()))
	assert.DeepEqual(t, store.credentials[provider.Host], credential)
}

func TestManagerRefreshesAndPersistsCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/token")
		assert.Equal(t, r.Header.Get("Authorization"), "")
		assert.NilError(t, r.ParseForm())
		assert.Equal(t, r.Form.Get("client_id"), "client-id")
		assert.Equal(t, r.Form.Get("grant_type"), "refresh_token")
		assert.Equal(t, r.Form.Get("refresh_token"), "old-refresh-token")
		w.Header().Set("Content-Type", "application/json")
		assert.NilError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token":             "new-access-token",
			"token_type":               "bearer",
			"refresh_token":            "new-refresh-token",
			"expires_in":               3600,
			"refresh_token_expires_in": 7200,
		}))
	}))
	defer server.Close()

	provider := testProvider(server.URL)
	store := newMemoryStore()
	store.credentials[provider.Host] = Credential{
		Account:      "octocat",
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		Expiry:       time.Now().Add(-time.Hour),
	}
	manager := NewManager(store, NewRegistry(provider), nil)

	token, err := manager.AccessToken(context.Background(), provider.Host)
	assert.NilError(t, err)
	assert.Equal(t, token, "new-access-token")
	assert.Equal(t, store.credentials[provider.Host].RefreshToken, "new-refresh-token")
	assert.Assert(t, store.credentials[provider.Host].Expiry.After(time.Now()))
	assert.Assert(t, store.credentials[provider.Host].RefreshExpiry.After(time.Now()))
}

func TestManagerStatusValidatesIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/user")
		assert.Equal(t, r.Header.Get("Authorization"), "Bearer access-token")
		assert.Equal(t, r.Header.Get("User-Agent"), userAgent)
		w.Header().Set("Content-Type", "application/json")
		assert.NilError(t, json.NewEncoder(w).Encode(map[string]string{"login": "octocat"}))
	}))
	defer server.Close()

	provider := testProvider(server.URL)
	store := newMemoryStore()
	store.credentials[provider.Host] = Credential{
		Account:     "old-account",
		AccessToken: "access-token",
		Expiry:      time.Now().Add(time.Hour),
	}
	manager := NewManager(store, NewRegistry(provider), server.Client())

	credential, err := manager.Status(context.Background(), provider.Host)
	assert.NilError(t, err)
	assert.Equal(t, credential.Account, "octocat")
	assert.Equal(t, store.credentials[provider.Host].Account, "octocat")
}

func TestManagerReportsMissingCredential(t *testing.T) {
	manager := NewManager(newMemoryStore(), DefaultRegistry(), nil)

	_, err := manager.AccessToken(context.Background(), "github.com")
	assert.Assert(t, errors.Is(err, ErrCredentialNotFound))
}

func testProvider(serverURL string) Provider {
	return Provider{
		Name:     "Test",
		Host:     "example.com",
		ClientID: "client-id",
		Endpoint: oauth2.Endpoint{
			DeviceAuthURL: serverURL + "/device",
			TokenURL:      serverURL + "/token",
			AuthStyle:     oauth2.AuthStyleInParams,
		},
		IdentityURL:  serverURL + "/user",
		AccountField: "login",
	}
}

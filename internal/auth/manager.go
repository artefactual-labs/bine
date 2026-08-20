package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/oauth2"
)

const userAgent = "bine"

// Manager coordinates providers and persisted credentials.
type Manager struct {
	store    Store
	registry *Registry
	client   *http.Client
}

// NewManager creates an authentication manager.
func NewManager(store Store, registry *Registry, client *http.Client) *Manager {
	if store == nil {
		store = NewKeyringStore("bine")
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	if client == nil {
		client = http.DefaultClient
	}

	return &Manager{store: store, registry: registry, client: client}
}

// NewDefaultManager creates an authentication manager backed by the system
// credential store.
func NewDefaultManager() *Manager {
	return NewManager(NewKeyringStore("bine"), DefaultRegistry(), nil)
}

// Providers returns the supported authentication providers.
func (m *Manager) Providers() []Provider {
	return m.registry.Providers()
}

// BeginDeviceAuth starts the OAuth device authorization flow.
func (m *Manager) BeginDeviceAuth(ctx context.Context, host string) (Provider, *oauth2.DeviceAuthResponse, error) {
	provider, err := m.registry.Lookup(host)
	if err != nil {
		return Provider{}, nil, err
	}

	config := provider.OAuthConfig()
	response, err := config.DeviceAuth(ctx)
	if err != nil {
		return Provider{}, nil, fmt.Errorf("start %s device authorization: %w", provider.Name, err)
	}

	return provider, response, nil
}

// CompleteDeviceAuth waits for device authorization, verifies the account,
// and saves the resulting credential.
func (m *Manager) CompleteDeviceAuth(
	ctx context.Context,
	provider Provider,
	response *oauth2.DeviceAuthResponse,
) (Credential, error) {
	config := provider.OAuthConfig()
	token, err := config.DeviceAccessToken(ctx, response)
	if err != nil {
		return Credential{}, fmt.Errorf("complete %s device authorization: %w", provider.Name, err)
	}

	account, err := m.identity(ctx, provider, token.AccessToken)
	if err != nil {
		return Credential{}, err
	}

	credential := credentialFromToken(token, account, time.Time{})
	if err := m.store.Set(provider.Host, credential); err != nil {
		return Credential{}, fmt.Errorf("save credential for %q: %w", provider.Host, err)
	}

	return credential, nil
}

// AccessToken returns a valid access token for host, refreshing and saving it
// when necessary.
func (m *Manager) AccessToken(ctx context.Context, host string) (string, error) {
	provider, err := m.registry.Lookup(host)
	if err != nil {
		return "", err
	}

	credential, err := m.credential(ctx, provider)
	if err != nil {
		return "", err
	}

	return credential.AccessToken, nil
}

// Status validates and returns the saved credential for host.
func (m *Manager) Status(ctx context.Context, host string) (Credential, error) {
	provider, err := m.registry.Lookup(host)
	if err != nil {
		return Credential{}, err
	}

	credential, err := m.credential(ctx, provider)
	if err != nil {
		return Credential{}, err
	}

	account, err := m.identity(ctx, provider, credential.AccessToken)
	if err != nil {
		return Credential{}, err
	}
	if account != credential.Account {
		credential.Account = account
		if err := m.store.Set(provider.Host, credential); err != nil {
			return Credential{}, fmt.Errorf("save credential for %q: %w", provider.Host, err)
		}
	}

	return credential, nil
}

// Logout removes the saved credential for host.
func (m *Manager) Logout(host string) error {
	provider, err := m.registry.Lookup(host)
	if err != nil {
		return err
	}

	return m.store.Delete(provider.Host)
}

func (m *Manager) credential(ctx context.Context, provider Provider) (Credential, error) {
	credential, err := m.store.Get(provider.Host)
	if err != nil {
		return Credential{}, err
	}

	token := credential.token()
	if token.Valid() {
		return credential, nil
	}
	if token.RefreshToken == "" {
		return Credential{}, fmt.Errorf("credential for %q has expired; log in again", provider.Host)
	}
	if !credential.RefreshExpiry.IsZero() && time.Now().After(credential.RefreshExpiry) {
		return Credential{}, fmt.Errorf("credential for %q has expired; log in again", provider.Host)
	}

	config := provider.OAuthConfig()
	refreshed, err := config.TokenSource(ctx, token).Token()
	if err != nil {
		return Credential{}, fmt.Errorf("refresh credential for %q: %w", provider.Host, err)
	}

	credential = credentialFromToken(refreshed, credential.Account, credential.RefreshExpiry)
	if err := m.store.Set(provider.Host, credential); err != nil {
		return Credential{}, fmt.Errorf("save refreshed credential for %q: %w", provider.Host, err)
	}

	return credential, nil
}

func (m *Manager) identity(ctx context.Context, provider Provider, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.IdentityURL, nil)
	if err != nil {
		return "", fmt.Errorf("create %s identity request: %w", provider.Name, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", userAgent)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %s identity: %w", provider.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s identity API returned status %d", provider.Name, resp.StatusCode)
	}

	var identity map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return "", fmt.Errorf("decode %s identity: %w", provider.Name, err)
	}
	var account string
	if err := json.Unmarshal(identity[provider.AccountField], &account); err != nil || account == "" {
		return "", errors.New("identity response has no account name")
	}

	return account, nil
}

func credentialFromToken(token *oauth2.Token, account string, previousRefreshExpiry time.Time) Credential {
	refreshExpiry := previousRefreshExpiry
	if seconds, ok := secondsFromExtra(token.Extra("refresh_token_expires_in")); ok {
		refreshExpiry = time.Now().Add(time.Duration(seconds) * time.Second)
	}

	return Credential{
		Account:       account,
		AccessToken:   token.AccessToken,
		TokenType:     token.TokenType,
		RefreshToken:  token.RefreshToken,
		Expiry:        token.Expiry,
		RefreshExpiry: refreshExpiry,
	}
}

func secondsFromExtra(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		return int64(value), true
	case json.Number:
		seconds, err := value.Int64()
		return seconds, err == nil
	case string:
		seconds, err := strconv.ParseInt(value, 10, 64)
		return seconds, err == nil
	default:
		return 0, false
	}
}

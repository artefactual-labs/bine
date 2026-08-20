package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

var (
	// ErrCredentialNotFound indicates that no credential exists for a host.
	ErrCredentialNotFound = errors.New("credential not found")
	// ErrCredentialStoreUnavailable indicates that the platform credential
	// store could not be accessed.
	ErrCredentialStoreUnavailable = errors.New("credential store unavailable")
)

// Credential is the persisted authentication state for a host.
type Credential struct {
	Account       string    `json:"account"`
	AccessToken   string    `json:"access_token"`
	TokenType     string    `json:"token_type,omitempty"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	Expiry        time.Time `json:"expiry,omitzero"`
	RefreshExpiry time.Time `json:"refresh_expiry,omitzero"`
}

func (c Credential) token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  c.AccessToken,
		TokenType:    c.TokenType,
		RefreshToken: c.RefreshToken,
		Expiry:       c.Expiry,
	}
}

// Store persists credentials by service host.
type Store interface {
	Get(host string) (Credential, error)
	Set(host string, credential Credential) error
	Delete(host string) error
}

// KeyringStore persists credentials in the operating system's credential
// store.
type KeyringStore struct {
	service string
}

// NewKeyringStore creates an operating-system credential store.
func NewKeyringStore(service string) *KeyringStore {
	return &KeyringStore{service: service}
}

// Get retrieves the credential for host.
func (s *KeyringStore) Get(host string) (Credential, error) {
	value, err := keyring.Get(s.service, host)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return Credential{}, ErrCredentialNotFound
		}
		return Credential{}, fmt.Errorf("%w: %w", ErrCredentialStoreUnavailable, err)
	}

	var credential Credential
	if err := json.Unmarshal([]byte(value), &credential); err != nil {
		return Credential{}, fmt.Errorf("decode credential for %q: %w", host, err)
	}
	if credential.AccessToken == "" {
		return Credential{}, fmt.Errorf("credential for %q has no access token", host)
	}

	return credential, nil
}

// Set saves the credential for host.
func (s *KeyringStore) Set(host string, credential Credential) error {
	value, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode credential for %q: %w", host, err)
	}
	if err := keyring.Set(s.service, host, string(value)); err != nil {
		return fmt.Errorf("%w: %w", ErrCredentialStoreUnavailable, err)
	}

	return nil
}

// Delete removes the credential for host.
func (s *KeyringStore) Delete(host string) error {
	if err := keyring.Delete(s.service, host); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrCredentialNotFound
		}
		return fmt.Errorf("%w: %w", ErrCredentialStoreUnavailable, err)
	}

	return nil
}

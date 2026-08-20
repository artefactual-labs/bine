package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"gotest.tools/v3/assert"
)

func TestKeyringStore(t *testing.T) {
	keyring.MockInit()
	store := NewKeyringStore("bine-test")
	want := Credential{
		Account:       "octocat",
		AccessToken:   "access-token",
		TokenType:     "bearer",
		RefreshToken:  "refresh-token",
		Expiry:        time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
		RefreshExpiry: time.Date(2027, time.February, 20, 12, 0, 0, 0, time.UTC),
	}

	assert.NilError(t, store.Set("github.com", want))
	got, err := store.Get("github.com")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, want)

	assert.NilError(t, store.Delete("github.com"))
	_, err = store.Get("github.com")
	assert.Assert(t, errors.Is(err, ErrCredentialNotFound))
}

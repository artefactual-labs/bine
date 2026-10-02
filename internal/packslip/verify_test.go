package packslip

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"gotest.tools/v3/assert"
)

func testRoot(t *testing.T) *root.TrustedRoot {
	t.Helper()
	r, err := root.NewTrustedRootFromPath("testdata/trusted-root.json")
	assert.NilError(t, err)
	return r
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	assert.NilError(t, err)
	return data
}

func TestVerifyPublishedBundles(t *testing.T) {
	for _, name := range []string{"packslip-v1.4.0.sigstore.json", "hk-v2.4.0.sigstore.json"} {
		t.Run(name, func(t *testing.T) {
			payload, id, err := VerifyBundle(fixture(t, name), testRoot(t))
			assert.NilError(t, err)
			assert.Assert(t, len(payload) > 0)
			assert.Assert(t, id.RepositoryID != "")
			assert.Equal(t, id.OwnerID, "216188")
			assert.Assert(t, strings.HasPrefix(id.Signer, id.Repository+"/.github/workflows/"))
			if strings.HasPrefix(name, "hk") {
				s, err := Parse(payload)
				assert.NilError(t, err)
				assert.Equal(t, s.Predicate.Project, "github.com/jdx/hk")
			} else {
				// A valid signature cannot make a domain project a GitHub project.
				_, err := Parse(payload)
				assert.ErrorContains(t, err, "github.com/owner/repo")
			}
		})
	}
}

func TestVerifyRejectsTamperingAndMissingLog(t *testing.T) {
	for _, change := range []string{"payload", "signature", "log", "identity"} {
		t.Run(change, func(t *testing.T) {
			var b map[string]any
			assert.NilError(t, json.Unmarshal(fixture(t, "hk-v2.4.0.sigstore.json"), &b))
			envelope := b["dsseEnvelope"].(map[string]any)
			switch change {
			case "payload", "identity":
				payload, err := base64.StdEncoding.DecodeString(envelope["payload"].(string))
				assert.NilError(t, err)
				text := string(payload)
				if change == "payload" {
					text = strings.ReplaceAll(text, "2.4.0", "2.4.1")
				} else {
					text = strings.ReplaceAll(text, "jdx/hk", "attacker/hk")
				}
				envelope["payload"] = base64.StdEncoding.EncodeToString([]byte(text))
			case "signature":
				envelope["signatures"].([]any)[0].(map[string]any)["sig"] = base64.StdEncoding.EncodeToString(make([]byte, 64))
			case "log":
				b["verificationMaterial"].(map[string]any)["tlogEntries"] = []any{}
			}
			data, err := json.Marshal(b)
			assert.NilError(t, err)
			_, _, err = VerifyBundle(data, testRoot(t))
			assert.Assert(t, err != nil)
		})
	}
}

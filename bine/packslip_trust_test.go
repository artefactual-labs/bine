package bine

import (
	"context"
	"crypto"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/artefactual-labs/bine/internal/packslip"
)

func TestPackslipGetKeepsTrustReadOnly(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		name := "writable storage"
		if readOnly {
			name = "read-only storage"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			assert.NilError(t, os.WriteFile(".bine.toml", []byte(`project = "test"
[[bins]]
name = "tool"
version = "1.2.3"
packslip = { project = "github.com/example/tool" }
`), 0o600))
			stateDir := filepath.Join(dir, "state")
			b, err := NewWithOptions(WithCacheDir(filepath.Join(dir, "cache")), WithStateDir(stateDir))
			assert.NilError(t, err)
			assert.NilError(t, os.MkdirAll(b.BinDir, 0o700))
			binPath := filepath.Join(b.BinDir, "tool")
			assert.NilError(t, os.WriteFile(binPath, []byte("installed executable"), 0o700))
			sum, err := checksum(binPath)
			assert.NilError(t, err)

			// Seed a persisted receipt and its separately accepted publisher. Get
			// must parse and validate both without contacting the publisher.
			release := packslip.Resolved{Identity: packslip.Identity{
				Repository: "https://github.com/example/tool", RepositoryID: "123", OwnerID: "456",
				Signer: "https://github.com/example/tool/.github/workflows/release.yml@refs/tags/v1.2.3",
			}}
			r := &packslip.Resolver{StateDir: stateDir}
			assert.NilError(t, r.Accept("github.com/example/tool", &release))
			marker, err := json.Marshal(versionMarkerDocument{
				Checksum: versionMarkerChecksum{Algorithm: crypto.SHA256.String(), Value: sum},
				Packslip: &packslipReceipt{
					Request: packslip.Request{Project: "github.com/example/tool", Version: "1.2.3", Name: "tool"},
					Release: release, ExecutableSHA256: sum,
				},
			})
			assert.NilError(t, err)
			assert.NilError(t, os.MkdirAll(filepath.Join(b.VersionsDir, "tool"), 0o700))
			assert.NilError(t, os.WriteFile(filepath.Join(b.VersionsDir, "tool", "1.2.3"), marker, 0o600))

			trustFile := filepath.Join(stateDir, "trust.json")
			before, err := os.ReadFile(trustFile)
			assert.NilError(t, err)
			oldTime := time.Unix(1234567890, 0)
			assert.NilError(t, os.Chtimes(trustFile, oldTime, oldTime))
			beforeInfo, err := os.Stat(trustFile)
			assert.NilError(t, err)
			assert.NilError(t, os.Remove(filepath.Join(stateDir, "trust.lock")))
			if readOnly {
				t.Cleanup(func() { assert.NilError(t, os.Chmod(stateDir, 0o700)) })
				assert.NilError(t, os.Chmod(trustFile, 0o400))
				assert.NilError(t, os.Chmod(stateDir, 0o500))
				probe, err := os.CreateTemp(stateDir, "permission-probe-*")
				if err == nil {
					assert.NilError(t, probe.Close())
					assert.NilError(t, os.Remove(probe.Name()))
					t.Skip("filesystem permits writes despite directory permissions")
				}
				assert.Assert(t, os.IsPermission(err), "unexpected permission probe failure: %v", err)
			}

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			for range 2 {
				path, err := b.Get(ctx, "tool")
				assert.NilError(t, err)
				assert.Equal(t, path, binPath)
			}
			after, err := os.ReadFile(trustFile)
			assert.NilError(t, err)
			assert.DeepEqual(t, after, before)
			afterInfo, err := os.Stat(trustFile)
			assert.NilError(t, err)
			assert.Equal(t, afterInfo.ModTime(), beforeInfo.ModTime())
			entries, err := os.ReadDir(stateDir)
			assert.NilError(t, err)
			assert.Equal(t, len(entries), 1)
			assert.Equal(t, entries[0].Name(), "trust.json")
		})
	}
}

package packslip

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestTrustInspectionDoesNotCreateState(t *testing.T) {
	r, request, _ := publishedResolver(t)
	r.StateDir = filepath.Join(r.StateDir, "missing")
	result := &Resolved{}
	accepted, err := r.Accepted(request.Project, result)
	assert.NilError(t, err)
	assert.Assert(t, !accepted)
	_, err = os.Stat(r.StateDir)
	assert.Assert(t, os.IsNotExist(err))

	result, err = r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	latest, err := r.Latest(t.Context(), request)
	assert.NilError(t, err)
	assert.Equal(t, latest, request.Version)
	accepted, err = r.Accepted(request.Project, result)
	assert.NilError(t, err)
	assert.Assert(t, !accepted, "discovery must not establish trust")
	_, err = os.Stat(r.StateDir)
	assert.Assert(t, os.IsNotExist(err))
}

func TestTrustInspectionPreservesState(t *testing.T) {
	r, request, _ := publishedResolver(t)
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	assert.NilError(t, r.Accept(request.Project, result))
	unchanged := snapshotTrust(t, r.StateDir)
	assert.NilError(t, os.Remove(filepath.Join(r.StateDir, "trust.lock")))
	for range 2 {
		accepted, err := r.Accepted(request.Project, result)
		assert.NilError(t, err)
		assert.Assert(t, accepted)
		_, err = r.Resolve(t.Context(), request)
		assert.NilError(t, err)
		_, err = r.Latest(t.Context(), request)
		assert.NilError(t, err)
	}
	unchanged()
	entries, err := os.ReadDir(r.StateDir)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1)
	assert.Equal(t, entries[0].Name(), "trust.json")
}

func TestAcceptPreservesUnchangedTrust(t *testing.T) {
	r, request, _ := publishedResolver(t)
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	assert.NilError(t, r.Accept(request.Project, result))
	unchanged := snapshotTrust(t, r.StateDir)
	for range 2 {
		assert.NilError(t, r.Accept(request.Project, result))
	}
	unchanged()
}

func TestAcceptPersistsStrongerTrust(t *testing.T) {
	r, request, _ := publishedResolver(t)
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	withoutProvenance := *result
	withoutProvenance.Artifact.Provenance = nil
	assert.NilError(t, r.Accept(request.Project, &withoutProvenance))
	assert.Assert(t, len(result.Artifact.Provenance) > 0)
	assert.NilError(t, r.Accept(request.Project, result))
	other := &Resolver{StateDir: r.StateDir, Host: r.Host}
	accepted, err := other.Accepted(request.Project, &withoutProvenance)
	assert.ErrorContains(t, err, "dropped previously declared provenance")
	assert.Assert(t, !accepted)
	assert.ErrorContains(t, other.Accept(request.Project, &withoutProvenance), "dropped previously declared provenance")
}

func TestTrustRejectsInvalidState(t *testing.T) {
	for _, data := range []string{
		`{`,
		`null`,
		`{}`,
		`{"version":1}`,
		`{"projects":{}}`,
		`{"version":null,"projects":{}}`,
		`{"version":2,"projects":{}}`,
		`{"version":1,"projects":null}`,
	} {
		t.Run(data, func(t *testing.T) {
			r, request, _ := publishedResolver(t)
			result, err := r.Resolve(t.Context(), request)
			assert.NilError(t, err)
			assert.NilError(t, r.Accept(request.Project, result))
			changed := *result
			changed.Identity.OwnerID = "new-owner"
			assert.ErrorContains(t, r.Accept(request.Project, &changed), "owner changed")
			// An incomplete existing file must not silently turn a remembered
			// publisher into first-use trust and permit a different owner.
			assert.NilError(t, os.WriteFile(filepath.Join(r.StateDir, "trust.json"), []byte(data), 0o600))
			unchanged := snapshotTrust(t, r.StateDir)
			accepted, err := r.Accepted(request.Project, result)
			assert.Check(t, err != nil)
			assert.Assert(t, !accepted)
			_, err = r.Resolve(t.Context(), request)
			assert.Check(t, err != nil)
			assert.Check(t, r.Accept(request.Project, &changed) != nil)
			unchanged()
		})
	}
}

func TestAcceptValidEmptyTrustState(t *testing.T) {
	r, request, _ := publishedResolver(t)
	assert.NilError(t, os.WriteFile(filepath.Join(r.StateDir, "trust.json"), []byte(`{"version":1,"projects":{}}`), 0o600))
	result, err := r.Resolve(t.Context(), request)
	assert.NilError(t, err)
	accepted, err := r.Accepted(request.Project, result)
	assert.NilError(t, err)
	assert.Assert(t, !accepted)
	assert.NilError(t, r.Accept(request.Project, result))
	accepted, err = r.Accepted(request.Project, result)
	assert.NilError(t, err)
	assert.Assert(t, accepted)
}

// Give the file a distinctive timestamp so an unnecessary rewrite is observable
// without sleeps or assumptions about the filesystem's timestamp resolution.
func snapshotTrust(t *testing.T, dir string) func() {
	t.Helper()
	file := filepath.Join(dir, "trust.json")
	data, err := os.ReadFile(file)
	assert.NilError(t, err)
	oldTime := time.Unix(1234567890, 0)
	assert.NilError(t, os.Chtimes(file, oldTime, oldTime))
	info, err := os.Stat(file)
	assert.NilError(t, err)
	return func() {
		t.Helper()
		after, err := os.ReadFile(file)
		assert.NilError(t, err)
		assert.DeepEqual(t, after, data)
		afterInfo, err := os.Stat(file)
		assert.NilError(t, err)
		assert.Equal(t, afterInfo.ModTime(), info.ModTime())
	}
}

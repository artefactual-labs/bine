package packslip

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"
	"github.com/rogpeppe/go-internal/lockedfile"
)

type pin struct {
	Identity
	Tool       string          `json:"tool,omitempty"`
	Provenance map[string]bool `json:"provenance,omitempty"`
}

// The state is separate from install receipts: reinstalling and cache cleanup
// must not reset first-use identity pins. Acceptance holds a lock across
// read/check/write in concurrent bine processes. An old software version cannot
// weaken a pin.
type trustState struct {
	Version  int            `json:"version"`
	Projects map[string]pin `json:"projects"`
}

// readTrust does not lock or create files. Writers replace trust.json atomically,
// so inspection sees a complete snapshot even during acceptance. Accept must
// still reload and recheck under the writer lock before changing a pin.
func (r *Resolver) readTrust() (*trustState, error) {
	if r.StateDir == "" {
		return nil, errors.New("packslip trust directory is not configured")
	}
	data, err := os.ReadFile(filepath.Join(r.StateDir, "trust.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &trustState{Version: 1, Projects: map[string]pin{}}, nil
	}
	if err != nil {
		return nil, err
	}
	// Only a missing file permits first-use defaults. Existing state must
	// declare its schema and projects, or corruption could reset trust.
	var s trustState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("read Packslip trust: %w", err)
	}
	if s.Version != 1 || s.Projects == nil {
		return nil, errors.New("unsupported or invalid Packslip trust state")
	}
	return &s, nil
}

func workflow(id Identity) string {
	return strings.SplitN(strings.TrimPrefix(id.Signer, id.Repository+"/"), "@", 2)[0]
}

func provenanceKey(host Host, a Artifact) string {
	// Bind continuity to the installation target, not publisher-controlled
	// platform annotations. Dropping an arch/libc field must not erase history.
	return strings.Join([]string{host.OS, host.Arch, a.Variant}, "/")
}

func (s *trustState) check(project string, id Identity, a Artifact, host Host) error {
	_, tool, err := Project(project)
	if err != nil {
		return err
	}
	key := provenanceKey(host, a)
	hadProvenance := false
	for name, old := range s.Projects {
		if name != project && (old.RepositoryID != id.RepositoryID || old.Tool != tool) {
			continue
		}
		if old.RepositoryID != id.RepositoryID {
			return errors.New("packslip repository ID changed; refusing a different repository under the same name")
		}
		if old.OwnerID != id.OwnerID {
			return errors.New("packslip repository owner changed; review the transfer before changing trust.json")
		}
		if workflow(old.Identity) != workflow(id) {
			return errors.New("packslip signing workflow changed; review the signer before changing trust.json")
		}
		hadProvenance = hadProvenance || old.Provenance[key]
	}
	if hadProvenance && len(a.Provenance) == 0 {
		return errors.New("packslip artifact dropped previously declared provenance")
	}
	return nil
}

func (r *Resolver) Accept(project string, result *Resolved) error {
	if r.StateDir == "" {
		return errors.New("packslip trust directory is not configured")
	}
	if err := os.MkdirAll(r.StateDir, 0o700); err != nil {
		return err
	}
	unlock, err := lockedfile.MutexAt(filepath.Join(r.StateDir, "trust.lock")).Lock()
	if err != nil {
		return err
	}
	defer unlock()
	s, err := r.readTrust()
	if err != nil {
		return err
	}
	if err := s.check(project, result.Identity, result.Artifact, r.Host); err != nil {
		return err
	}
	_, tool, _ := Project(project)
	p := pin{Identity: result.Identity, Tool: tool, Provenance: map[string]bool{}}
	for name, old := range s.Projects {
		if name == project || (old.RepositoryID == p.RepositoryID && old.Tool == tool) {
			maps.Copy(p.Provenance, old.Provenance)
		}
	}
	a := result.Artifact
	if len(a.Provenance) > 0 {
		p.Provenance[provenanceKey(r.Host, a)] = true
	}
	if old, ok := s.Projects[project]; ok && old.Identity == p.Identity && old.Tool == p.Tool && maps.Equal(old.Provenance, p.Provenance) {
		return nil
	}
	s.Projects[project] = p
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return renameio.WriteFile(filepath.Join(r.StateDir, "trust.json"), data, 0o600, renameio.WithStaticPermissions(0o600))
}

// Accepted validates an offline receipt against durable trust. Removing trust
// state forces a fresh verified install instead of trusting a cache by itself.
func (r *Resolver) Accepted(project string, result *Resolved) (bool, error) {
	s, err := r.readTrust()
	if err != nil {
		return false, err
	}
	if _, ok := s.Projects[project]; !ok {
		return false, nil
	}
	if err := s.check(project, result.Identity, result.Artifact, r.Host); err != nil {
		return false, err
	}
	return true, nil
}

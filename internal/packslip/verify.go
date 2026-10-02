// Package packslip resolves signed release metadata for standalone executables.
package packslip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const githubIssuer = "https://token.actions.githubusercontent.com"

// Identity contains only authenticated certificate claims, never bundle hints.
type Identity struct {
	Signer       string `json:"signer"`
	Repository   string `json:"repository"`
	RepositoryID string `json:"repository_id"`
	OwnerID      string `json:"owner_id"`
}

// VerifyBundle checks the signature, certificate chain, certificate transparency
// and Rekor evidence. Callers must additionally match the project and version to
// their request and verify downloaded files against the statement's subjects.
func VerifyBundle(data []byte, trusted root.TrustedMaterial) ([]byte, Identity, error) {
	var b bundle.Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, Identity{}, fmt.Errorf("decode Sigstore bundle: %w", err)
	}
	if b.GetMediaType() != "application/vnd.dev.sigstore.bundle.v0.3+json" || b.GetDsseEnvelope().GetPayloadType() != "application/vnd.in-toto+json" {
		return nil, Identity{}, errors.New("expected a v0.3 Sigstore bundle containing an in-toto statement")
	}
	v, err := verify.NewVerifier(trusted, verify.WithTransparencyLog(1), verify.WithIntegratedTimestamps(1), verify.WithSignedCertificateTimestamps(1))
	if err != nil {
		return nil, Identity{}, err
	}
	policy, err := verify.NewShortCertificateIdentity(githubIssuer, "", "", `^https://github\.com/[^/]+/[^/]+/\.github/workflows/[^/@]+@refs/(tags|heads)/.+$`)
	if err != nil {
		return nil, Identity{}, err
	}
	// Artifact checking is performed after selection and download, before any
	// extraction. Here we authenticate the complete, unmodified DSSE payload.
	result, err := v.Verify(&b, verify.NewPolicy(verify.WithoutArtifactUnsafe(), verify.WithCertificateIdentity(policy)))
	if err != nil {
		return nil, Identity{}, fmt.Errorf("verify Sigstore bundle: %w", err)
	}
	if result.Signature == nil || result.Signature.Certificate == nil {
		return nil, Identity{}, errors.New("missing verified signing certificate")
	}
	c := result.Signature.Certificate
	id := Identity{c.SubjectAlternativeName, c.SourceRepositoryURI, c.SourceRepositoryIdentifier, c.SourceRepositoryOwnerIdentifier}
	if id.RepositoryID == "" || id.OwnerID == "" {
		return nil, Identity{}, errors.New("packslip requires a certificate with repository and owner IDs")
	}
	return b.GetDsseEnvelope().GetPayload(), id, nil
}

func trustedRoot(ctx context.Context, dir string) (*root.TrustedRoot, error) {
	return root.FetchTrustedRootWithOptions(tuf.DefaultOptions().WithContext(ctx).WithCachePath(dir).WithCacheValidity(1))
}

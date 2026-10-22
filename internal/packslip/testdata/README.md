# Packslip verification fixtures

These public release bundles are preserved byte-for-byte for offline signature
verification tests:

- `hk-v2.4.0.sigstore.json`:
  https://github.com/jdx/hk/releases/download/v2.4.0/packslip.sigstore.json
- `packslip-v1.4.0.sigstore.json`:
  https://github.com/jdx/packslip/releases/download/v1.4.0/packslip.sigstore.json

`trusted-root.json` was obtained on 2026-09-29 through sigstore-go v1.3.0's
authenticated TUF client, using its embedded public-good TUF root and
https://tuf-repo-cdn.sigstore.dev. It is only a deterministic test fixture;
production verification refreshes trust material through TUF.

The hk bundle identifies `github.com/jdx/hk`. The Packslip bundle identifies
`packslip.dev`, and is deliberately used to demonstrate that a valid GitHub
Actions signature does not turn a domain project into a GitHub project.

No artifacts or publisher executables are run by these tests.

## Upstream statement conformance

`statement-validity.json` is copied without modification from
[Packslip's shared conformance fixtures] at commit
`6a6ba5047de569786ca900d1a8ea6d94d32037e2`. `PACKSLIP-LICENSE` preserves its
upstream MIT license. Tests read this local copy and do not require a network
connection. To update it, copy the fixture and license from a reviewed upstream
revision and update this commit reference.

All cases run through bine's public `Parse` entry point with their original
accept/reject expectations. These fixtures cover structural validity, not
signature verification, artifact selection, or installation.

Bine additionally restricts projects to GitHub and signing identities to vendor
GitHub Actions manifests, and enforces safe paths and HTTPS URLs. Separate
parser tests cover these restrictions. A valid `pin_workflow: false` declaration
parses, but does not relax bine's durable workflow pins; the conformance case
tests parsing only. Resource declarations are validated but never executed or
installed.

[packslip's shared conformance fixtures]: https://github.com/jdx/packslip/blob/6a6ba5047de569786ca900d1a8ea6d94d32037e2/tests/conformance/statement-validity.json

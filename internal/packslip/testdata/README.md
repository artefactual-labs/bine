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

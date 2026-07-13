# Open-source launch checklist

Last reviewed: 2026-07-13. Owner: release maintainer. A release may be promoted only when each advertised claim has
both a normative reference and executable evidence.

- Public checkout: `make clean-room-check`, anonymous repository/tag checks, licenses, and compatibility matrix.
- Core request path: quickstart request and `make check`; expected status/body verified.
- Optional profiles: runtime-admin, receipt, and HA owned-infrastructure smoke/failure drills.
- Contracts: Request, Admin, Receipt, CLI, config, metrics, errors, and compatibility references pass drift checks.
- Security: threat-model invariant tests and green Go/npm license, dependency, CodeQL, govulncheck, secret, and OCI scans;
  after changing visibility, enable and verify GitHub native secret scanning and push protection before accepting changes.
- Artifacts: protected release workflow produces checksums, module inventory, SBOM, provenance, attestations,
  signatures, multi-architecture images, notes, and draft release.
- Operations: upgrade/rollback, backup/restore, hardening, diagnostics, and support routes reviewed.
- Post-publish: download binaries, verify checksums/attestations/signatures, pull images by digest, run request/profile
  smoke tests, verify docs and release links, then publish rather than leave draft/prerelease.

Prerelease exceptions must name an owner, affected claim, compensating warning, expiry, and issue. No exception may
hide private dependencies, credential exposure, an unverified artifact, or a broken default quickstart.

# Release checklist

A release may be promoted only when each advertised claim has both a normative reference and executable evidence.

- Source checkout: `make clean-room-check`, anonymous repository and tag checks, licenses, and compatibility matrix.
- Core request path: quickstart request and `make check`; expected status and body verified.
- Optional profiles: runtime administration, receipt, and HA smoke tests and failure drills on owned infrastructure.
- Contracts: Request, Admin, Receipt, CLI, configuration, metrics, errors, and compatibility references pass drift checks.
- Security: threat-model invariant tests and green Go/npm license, dependency, CodeQL, govulncheck, secret, and OCI scans;
  GitHub secret scanning and push protection enabled.
- Artifacts: release workflow produces checksums, module inventory, SBOM, provenance, attestations,
  signatures, multi-architecture images, notes, and a draft release.
- Operations: upgrade and rollback, backup and restore, hardening, diagnostics, and support routes reviewed.
- Post-publish: download binaries, verify checksums, attestations, and signatures, pull images by digest, run request
  and profile smoke tests, verify documentation and release links, then publish the draft.

Exceptions must name an owner, affected claim, compensating warning, expiry, and issue. No exception may hide a
dependency, credential exposure, an unverified artifact, or a broken default quickstart.

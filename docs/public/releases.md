# Release procedure

Releases are performed by a maintainer with access to the protected `release` environment. The workflow is automated;
repository visibility, environment approval, package settings, and post-publish observation are owner actions.

1. Confirm `make check production-deploy-check docs-website clean-room-check race` on `main` and a green security
   workflow. Confirm the launch checklist and compatibility matrix.
2. Release protocol source, Go/Python bindings, then Go/Python SDKs when their versions change. Update exact tags and
   locks here; run `make conformance` and trusted compatibility CI.
3. Move `Unreleased` notes to the chosen semantic version, document migrations and profile-specific upgrade order,
   then create signed tag `vX.Y.Z` from reviewed `main`.
4. The protected release workflow rebuilds checks, produces Linux/macOS amd64/arm64 binaries, checksums, module and
   Go and npm license inventories, attestations, multi-architecture OCI images, SBOM/provenance, keyless signatures, and a draft
   release. A dependency without a distributed license/notice file fails the release.
5. Verify checksums and attestations, pull images by digest, run the default request smoke test and enabled profile
   smoke tests, then publish the draft and documentation version.

Upgrade Egress before Control so old Control does not assign a capability newer workers cannot understand; then
upgrade CLI/clients. Back up JetStream runtime configuration and receipt records/objects before stateful upgrades.
Rollback in reverse order using immutable binary/image digests and restore state only when release notes require a
data-format rollback. A failed post-publish smoke test leaves the release draft/prerelease and triggers rollback.

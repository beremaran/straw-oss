# Release procedure

Operators installing a published build should start with [Install a release](installation.md). This page is the
maintainer procedure for producing and promoting a release.

Releases are performed by a maintainer with access to the `release` environment. The workflow is automated;
environment approval, package settings, and post-publish observation are owner actions.

1. Confirm `make check production-deploy-check docs-website clean-room-check race` on `main` and a green security
   workflow. Confirm the release checklist and compatibility matrix.
2. Release protocol source, Go/Python bindings, then Go/Python SDKs when their versions change. Update exact public tags
   and locks here; run `make conformance` and the executable old/new decoder workflow below. Cross-repository release
   dispatch uses the maintainer-managed `STRAW_AUTOMATION_TOKEN`; normal CI and contributor pull requests require no
   repository secret.
3. Move `Unreleased` notes to the chosen semantic version, document migrations and profile-specific upgrade order,
   then create signed tag `vX.Y.Z` from reviewed `main`.
4. The release workflow rebuilds checks, produces Linux/macOS amd64/arm64 binaries, checksums, module and
   Go and npm license inventories, attestations, multi-architecture OCI images, SBOM/provenance, keyless signatures, and a draft
   release. A dependency without a distributed license/notice file fails the release.
5. Verify checksums and attestations, pull images by digest, run the default request smoke test and enabled profile
   smoke tests, then publish the draft and documentation version.

## Protocol minor-2 decoder evidence

`make conformance` and the repository JSON fixture checks prove fixture formatting, declaration, and current parsing;
they do not execute a previous tagged decoder and are insufficient evidence for additive field compatibility. The
release record must prove all four cells below against the canonical `v0.4.0` `streaming.json` fixture:

| Decoder | Input | Required assertion |
| --- | --- | --- |
| Go binding `v0.3.0` | new proxy `RequestStart` containing field 16 | decode succeeds, all fields 1-15 retain their values, and field 16 remains unknown to the old generated type |
| Python binding `v0.3.0` | new proxy `RequestStart` containing field 16 | decode succeeds, all fields 1-15 retain their values, and field 16 remains unknown to the old generated type |
| Go binding `v0.4.0` | old wire form without field 16 | decode succeeds and known fields retain their values |
| Python binding `v0.4.0` | old wire form without field 16 | decode succeeds and known fields retain their values |

Run the maintained cross-tag gate. It creates disposable Go modules and a Python environment pinned to the actual
`v0.3.0` bindings, feeds them the unmodified proxy fixture, and also runs `v0.4.0` decoders against the old direct wire:

```sh
make protocol-compatibility
```

The tagged compatibility workflow runs this target in addition to `make conformance`. Retain its command log as release
evidence; do not mark compatibility complete from JSON checks alone.

## Minor-2 runtime rollout

After protocol, binding, and SDK publication, deploy the new Control first while all pools remain direct. Remove every
old Control, then expire/delete shared worker rows after the final old writer stops. Deploy minor-2 workers against
direct pools and force fresh registration. Only then create fresh disabled proxy pool IDs, roll the intended workers
with exact profile claims, and enable a canary route. See [Operations](operations.md#roll-out-upstream-proxy-pools) for
the full validation sequence.

Rollback by disabling proxy routes/pools and selecting an existing or fresh direct pool. Never mutate a proxy pool into
direct execution under the same ID. Upgrade CLI/clients after the runtime. Back up JetStream runtime configuration and
receipt records/objects before stateful upgrades. Use immutable binary/image digests and restore state only when release
notes require a data-format rollback. A failed post-publish smoke test leaves the release draft/prerelease and triggers
rollback.

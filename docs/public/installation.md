# Install a release

The checked-in Compose files build Control and Egress from source. Use this page when installing a published release
from its GitHub release assets or container images.

## Choose an artifact

The release workflow publishes:

| Artifact | Platforms or contents |
| --- | --- |
| `straw-control_<os>_<arch>` | Control binary for `linux` or `darwin`, `amd64` or `arm64` |
| `straw-egress_<os>_<arch>` | Official Egress worker for the same four target combinations |
| `straw-straw_<os>_<arch>` | Public CLI for the same four target combinations |
| `SHA256SUMS` | SHA-256 checksums for the release files |
| `straw.cdx.json` | Go dependency SBOM; the release also includes module and license inventories |
| `ghcr.io/beremaran/straw-oss-control:<version>` | Multi-architecture Control image for `linux/amd64` and `linux/arm64` |
| `ghcr.io/beremaran/straw-oss-egress:<version>` | Multi-architecture Egress image for `linux/amd64` and `linux/arm64` |

The release tag is `vX.Y.Z`; the image tag uses the semantic version produced by the release workflow. Production
rollbacks should use a verified image digest or an exact binary checksum, not a moving tag.

## Download and verify a binary

Set the release version and target before downloading. The release asset URL uses the repository's tag:

```sh
version=0.1.0
os=linux
arch=amd64
asset="straw-control_${os}_${arch}"
base="https://github.com/beremaran/straw-oss/releases/download/v${version}"

curl -fLO "${base}/${asset}"
curl -fLO "${base}/SHA256SUMS"

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum --ignore-missing -c SHA256SUMS
else
  shasum -a 256 -c SHA256SUMS
fi

chmod +x "${asset}"
./"${asset}" -help
```

Download `straw-egress_${os}_${arch}` and `straw-straw_${os}_${arch}` in the same way when the deployment needs the
official worker or CLI. Keep the checksum file with the deployment record.

The release also carries GitHub build-provenance attestations. Verify a downloaded asset with the GitHub CLI before
installing it when your release policy requires provenance verification:

```sh
gh attestation verify "${asset}" --repo beremaran/straw-oss
```

## Pull and verify an image

Use the semver tag to inspect a release, then record and deploy the immutable digest:

```sh
version=0.1.0
image="ghcr.io/beremaran/straw-oss-control:${version}"

docker pull "$image"
digest=$(docker image inspect --format '{{index .RepoDigests 0}}' "$image")
printf 'record this digest: %s\n' "$digest"
docker pull "$digest"
```

Verify the signed image with the repository's GitHub Actions identity before promotion. Replace `IMAGE@DIGEST` with the
recorded digest and keep the identity constraint in the deployment evidence:

```sh
cosign verify \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'https://github.com/beremaran/straw-oss/.github/workflows/release.yml@refs/tags/v.*' \
  IMAGE@DIGEST
```

Repeat the pull and verification for `straw-oss-egress`. Adapt `deploy/production/compose.yml` to use the verified
`image:` references instead of its source `build:` blocks; keep the configuration, NATS, and optional profile guidance
from [Deployment](deployment.md).

## First run and upgrades

The default deployment still needs NATS, one Control, and one or more Egress workers. Set a deployment request token,
send it as `Authorization: Bearer <token>`, and keep the Control API behind a trusted reverse proxy or network policy.
Use [Configuration](configuration.md) for static JSON and [Deployment](deployment.md) for the optional
runtime-administration, receipt, TLS, and HA profiles.

For an upgrade, verify the new artifacts and back up JetStream or receipt state for enabled profiles. The protocol 1.2
upstream-proxy release requires Control first while pools remain direct, removal of all old Controls and shared worker
rows, and then minor-2 Egress workers; only then enable fresh proxy pool IDs. Upgrade the CLI and clients after the
runtime. Inspect readiness, worker availability, rollout state, and error rates before
retiring the previous digest. Roll back in reverse order using the previously verified artifacts; see
[Releases](releases.md) for the maintainer release procedure and [Operations](operations.md) for stateful backup drills.

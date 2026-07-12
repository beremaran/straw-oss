# Repository split completion report

Date: 2026-07-13

Status: technical cutover complete; final `straw-oss` owner review, merge, and default-branch activation remain.

All repositories and releases listed here are private. Public launch, external Python publication, public Go module
proxy availability, long-term compatibility policy, and coordinated public versioning remain deferred.

## Repository and release inventory

| Repository | Permanent identity | Private release | Release commit |
| --- | --- | --- | --- |
| `beremaran/straw-protos` | `straw.v1` | `v0.3.0` | `0c5613d82a785347dc2592f5ef0f373ced9cd0a8` |
| `beremaran/straw-protos-go` | `github.com/beremaran/straw-protos-go/straw/v1` | `v0.3.0` | `627b9a355be01c4c46a33e5e1c8da6bd8e6df03f` |
| `beremaran/straw-protos-python` | distribution `straw-protos`, import `straw_protos` | `v0.3.0` | `8b5bd437ad9c6f5eb29ea84dbccb8c54bdb74007` |
| `beremaran/straw-sdk-go` | `github.com/beremaran/straw-sdk-go` | `v0.1.0` | `db8916e994adb53f21978a30b28f7589b17e7d1c` |
| `beremaran/straw-sdk-python` | distribution `straw-sdk`, import `straw` | `v0.1.0` | `a56ab2f6db6c53ed543b600fe96b73b0851bccce` |
| `beremaran/straw-oss` | `github.com/beremaran/straw-oss` | independently versioned | repository-split pull request |

The Go `/v2` module path was corrected before extraction because it had never been released. The protocol repository
is the only canonical protocol source. Generated repositories record source and generator provenance and reproduce
their checked-in output from the exact `straw-protos` tag.

## Cutover result

- `straw-oss` has no local `api/proto`, `sdk`, `python`, or `examples/egress-static` tree.
- Go consumes exact `straw-protos-go v0.3.0` and `straw-sdk-go v0.1.0` module tags without `replace` or `go.work`.
- Python integration consumes exact private Git tags `straw-sdk-python v0.1.0` and transitively
  `straw-protos-python v0.3.0` through the committed root `uv.lock`; nothing is fetched from PyPI for these packages.
- the canonical Egress stays in `straw-oss` and exercises the external Go SDK's base worker machinery.
- every extracted project builds and tests without a sibling checkout or warm dependency cache.
- language-neutral fixtures cover signing, envelopes, negotiation, stream cancellation and credit, and body references.

## Verification evidence

The pre-split baseline and final branch both passed:

- `make check`;
- `make production-deploy-check`;
- `make docs-website`.

Standalone clean-cache checks passed for `straw-protos`, both generated-binding repositories, both SDK repositories,
and `straw-oss`. The final `straw-oss` run resolved all four private module/package dependencies from their remote
tags using new Go and uv caches. `scripts/verify-dependency-direction.sh` also passed.

Each of the five filtered repositories retained relevant authorship and history. Full retained-history secret scans
reported zero findings, file-boundary review found no unrelated cache or runtime trees, and no retained blob exceeded
1 MiB. License and attribution continuity use MIT licensing in all six repositories.

## Automation and compatibility

- private `straw-protos v0.3.0` dispatched exact-tag generation to both binding repositories;
- owner-reviewed generated pull requests produced matching private `v0.3.0` binding releases;
- binding release workflows dispatch exact-tag consumer updates through the dedicated GitHub App;
- manual idempotency runs of both SDK update workflows succeeded with `v0.3.0` and made no unnecessary pull request;
- `straw-oss` owns the current-version cross-repository compatibility matrix and an exact-tag Go binding updater;
- every third-party GitHub Action reference is pinned to a full commit SHA.

The dedicated GitHub App is installed only on these six repositories. Its repository permissions are limited to
metadata read, Actions write, contents write, and pull requests write; webhooks are disabled. App credentials exist
only in GitHub Actions variables and secrets.

## Repository controls and limitations

All six repositories were verified private. Issues are enabled, wiki and projects are disabled, squash merging is
enabled, merge commits and rebasing are disabled, and merged branches are deleted. Actions are limited to selected
GitHub-owned, verified, and explicitly allow-listed actions with read-only default workflow permissions; workflows
cannot approve pull requests.

GitHub's API refuses branch protection for private repositories owned by this personal account with `403 Upgrade to
GitHub Pro or make this repository public`. Making a repository public is outside the authorization boundary, so the
closest private enforcement is required green CI by process, pull-request-only changes, owner review, pinned Actions,
and no App permission to approve pull requests. The same account/repository combination returns `404` when enabling
Private Vulnerability Reporting. Each repository therefore includes `SECURITY.md` reporting instructions as the
available private fallback.

After the final owner-reviewed `straw-oss` pull request is merged, activation consists only of creating and selecting
`main` as the default branch, applying the same repository settings, running the now-default compatibility and updater
workflows once, and recording their green results. No repository may be made public during activation.

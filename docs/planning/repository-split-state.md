# Repository split execution state

The extraction freeze is lifted. The protocol, generated bindings, and SDKs now live in their permanent private
repositories, and `straw-oss` consumes their immutable private tags. The final cutover is ready for owner review and
activation by merging the repository-split pull request and changing the `straw-oss` default branch to `main`.

Pre-split baseline on 2026-07-13:

- `make check`: passed;
- `make production-deploy-check`: passed;
- `make docs-website`: passed.

The execution branch preserves the two local commits that preceded the split (`b83c220` and `18fec87`). The original
checkout is never used for destructive history filtering; extracted histories are built in separate temporary clones.

See [Repository split completion report](repository-split-completion.md) for the release, verification, automation,
and repository-control evidence.

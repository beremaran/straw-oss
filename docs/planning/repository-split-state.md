# Repository split execution state

The private repository split is active. Until the final completion audit passes, changes to `api/proto`, `sdk`, and
`python` are frozen unless they are part of the split itself. Runtime work outside those paths must not change
protocol or SDK surfaces.

Pre-split baseline on 2026-07-13:

- `make check`: passed;
- `make production-deploy-check`: passed;
- `make docs-website`: passed.

The execution branch preserves the two local commits that preceded the split (`b83c220` and `18fec87`). The original
checkout is never used for destructive history filtering; extracted histories are built in separate temporary clones.

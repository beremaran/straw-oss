---
sidebar_position: 12
---

# Development and releases

Read `CONTRIBUTING.md` before sending a change. The normal loop is:

```sh
make dev
go test ./internal/control -run TestName
make check
make production-deploy-check
make docs-website
```

Python development uses the root uv workspace:

```sh
uv sync --all-packages --frozen
uv run --all-packages --frozen python -m unittest discover python/tests
```

Do not create a nested virtual environment or lock file under `python/`.

## Release checklist

Maintainers release from a clean main branch:

1. update `CHANGELOG.md` and public docs;
2. run `make check`, `make production-deploy-check`, and `make docs-website`;
3. run the quickstart against the local Compose stack;
4. tag a semantic version such as `v0.2.0`;
5. publish release notes describing changes, compatibility, and upgrade steps;
6. publish container and SDK artifacts from the tag when release automation is configured.

Until the project reaches `v1.0.0`, minor releases may change advanced worker protocol surfaces. The REST request API
and documented configuration should still be changed deliberately and called out in the changelog.

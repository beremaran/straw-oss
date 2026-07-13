---
sidebar_position: 12
---

# Development and releases

Use `make help` for maintained commands. See the [verification strategy](test-strategy.md),
[compatibility policy](compatibility.md), [release procedure](releases.md), and
[documentation policy](documentation-policy.md). Governance and support are defined in the repository root.

Fast public CI is unprivileged. Trusted cross-repository compatibility and protected publishing run separately, so
pull-request code never receives maintainer credentials.

Read `CONTRIBUTING.md` before sending a change. The normal loop is:

```sh
make dev
go test ./internal/control -run TestName
make check
make production-deploy-check
make docs-website
```

Tagged Python SDK integration uses the root lock:

```sh
uv sync --frozen
uv run --frozen python -m unittest discover integration/python
```

Python SDK development and its independent lock live in `straw-sdk-python`.

The contributor handbook defines the enforced package graph, generated/external source ownership, risk-based test
selection, fixture workflow, and the complete public-contract checklist. Public contract changes must update tests,
the normative reference, compatibility classification, `CHANGELOG.md`, and representative executable evidence in
the same change.

The exact release graph and rollback procedure live in [Release procedure](releases.md); do not release from this
summary alone.

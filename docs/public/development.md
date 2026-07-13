---
sidebar_position: 12
---

# Development and releases

This page is the website entry point for contributors. The authoritative contributor handbook is
[`CONTRIBUTING.md`](https://github.com/beremaran/straw-oss/blob/main/CONTRIBUTING.md) in the repository root; read it
before sending a change. Governance and support policies also live in the root as
[`GOVERNANCE.md`](https://github.com/beremaran/straw-oss/blob/main/GOVERNANCE.md) and
[`SUPPORT.md`](https://github.com/beremaran/straw-oss/blob/main/SUPPORT.md).

## Set up

Required for all changes: Go at the exact version declared by `go.mod`, Docker with Compose v2, `make`, and
`golangci-lint`. Python changes also require Python 3.13 and uv; documentation-site changes require Node.js 20 or
later.

```sh
git clone https://github.com/beremaran/straw-oss.git
cd straw-oss
make dev
make check
```

Use `make help` for every maintained command. The default stack uses ports 4222, 8222, 8080, and 9090; see the
[deployment guide](deployment.md#local-development) for port overrides.

## Repository map

- `cmd/control`, `cmd/egress`, `cmd/straw`: process composition, flags, lifecycle, and health for the three binaries.
- `internal/control`, `internal/egress`: request pipeline and worker executor; runtime behavior belongs here, not in
  `cmd`.
- `internal/config`, `internal/natsx`, `internal/receipt`, `internal/objectstore`: configuration, NATS transport,
  receipt lifecycle, and object storage.
- `deploy/local`, `deploy/production`: supported development stack and the adaptable production example.
- `docs/public`, `website`: the shipped manual and the Docusaurus site that renders it.
- Protocol source, generated bindings, and the Go/Python SDKs live in their own tagged repositories listed in
  [Public components and provenance](components.md); never copy or hand-edit generated bindings here.

`make dependency-check` enforces the direct-import graph and exact external module pins.

## The normal loop

```sh
make dev
go test ./internal/control -run TestName
make check
make production-deploy-check
make docs-website
```

Choose broader evidence by risk — race, fuzz, conformance, profile, or HA drills — using the table in the
[verification strategy](test-strategy.md). Tagged Python SDK integration uses the root lock:

```sh
uv sync --frozen
uv run --frozen python -m unittest discover integration/python
```

Python SDK development and its independent lock live in `straw-sdk-python`.

## Change a public contract

Public contracts include static config, runtime snapshots, REST routes and JSON, stable errors, CLI flags/output,
metrics, protobuf/NATS messages, SDK behavior, and compatibility guarantees. Any such change must, in the same
reviewed change: add positive and negative tests, update the normative page under `docs/public`, classify the change
in `CHANGELOG.md`, pass `make public-surface-check`, and exercise a representative request or maintained example.
The [compatibility policy](compatibility.md) defines what each release type may change.

## CI and releases

Fast public CI is unprivileged. Trusted cross-repository compatibility and protected publishing run separately, so
pull-request code never receives maintainer credentials.

The exact release graph, artifact verification, upgrade order, and rollback procedure live in the
[release procedure](releases.md); do not release from this summary alone. Documentation changes follow the
[documentation policy](documentation-policy.md).

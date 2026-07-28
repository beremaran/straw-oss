# Components and provenance

Straw uses immutable Git tags across six independently versioned MIT-licensed repositories. The
clean-room gate checks anonymous access to every repository before resolving Go and Python dependency graphs.

| Component | Repository / immutable tag | Release commit | Purpose and provenance |
| --- | --- | --- | --- |
| Straw runtime | [`straw-oss`](https://github.com/beremaran/straw-oss) / release tag | release commit | Control, official Egress, CLI, deployment and manual |
| protocol source | [`straw-protos`](https://github.com/beremaran/straw-protos) / `v0.4.0` | `a4bb437e348027d39526c0c0e8489ff2c8b2aca2` | canonical protocol 1.2 protobuf and conformance producer |
| Go binding | [`straw-protos-go`](https://github.com/beremaran/straw-protos-go) / `v0.4.0` | `084f3d79d460e951efc12ef768efb0bc8895e13c` | reproducibly generated from protocol `v0.4.0` |
| Go SDK | [`straw-sdk-go`](https://github.com/beremaran/straw-sdk-go) / `v0.4.0` | `0a26c7cfb35c29f59f6f3f34c72fc3e6e5bfc1df` | REST client and protocol-minor-2 worker SDK |
| Python binding | [`straw-protos-python`](https://github.com/beremaran/straw-protos-python) / `v0.4.0` | `167495e66b349ffa4f951fa65c8108afbe031911` | reproducibly generated from protocol `v0.4.0` |
| Python SDK | [`straw-sdk-python`](https://github.com/beremaran/straw-sdk-python) / `v0.2.1` | `c59f4d1a049d87682994b6784b9eda82637ee0d3` | REST client and direct-only protocol-minor-1 worker SDK with additive minor-2 decoding |

Each tagged repository contains its own MIT `LICENSE`. `go.mod`, `pyproject.toml`, and `uv.lock` pin these exact tags;
the conformance manifest names producers and consumers. A release changing protocol source publishes source, generated
bindings, SDKs, then Straw in that order. Checksums, SBOMs, attestations, provenance, and signatures for Straw binaries
and images are attached by the release workflow.

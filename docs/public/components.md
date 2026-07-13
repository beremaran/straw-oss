# Components and provenance

Straw uses immutable Git tags across six independently versioned MIT-licensed repositories. The
clean-room gate checks anonymous access to every repository before resolving Go and Python dependency graphs.

| Component | Repository / immutable tag | Release commit | Purpose and provenance |
| --- | --- | --- | --- |
| Straw runtime | [`straw-oss`](https://github.com/beremaran/straw-oss) / release tag | release commit | Control, official Egress, CLI, deployment and manual |
| protocol source | [`straw-protos`](https://github.com/beremaran/straw-protos) / `v0.3.0` | `0c5613d82a785347dc2592f5ef0f373ced9cd0a8` | canonical `straw.v1` protobuf and conformance producer |
| Go binding | [`straw-protos-go`](https://github.com/beremaran/straw-protos-go) / `v0.3.0` | `627b9a355be01c4c46a33e5e1c8da6bd8e6df03f` | reproducibly generated from protocol `v0.3.0` |
| Go SDK | [`straw-sdk-go`](https://github.com/beremaran/straw-sdk-go) / `v0.1.0` | `db8916e994adb53f21978a30b28f7589b17e7d1c` | REST client and worker SDK |
| Python binding | [`straw-protos-python`](https://github.com/beremaran/straw-protos-python) / `v0.3.0` | `8b5bd437ad9c6f5eb29ea84dbccb8c54bdb74007` | reproducibly generated from protocol `v0.3.0` |
| Python SDK | [`straw-sdk-python`](https://github.com/beremaran/straw-sdk-python) / `v0.1.0` | `a56ab2f6db6c53ed543b600fe96b73b0851bccce` | `straw-sdk` REST client and worker SDK distribution |

Each tagged repository contains its own MIT `LICENSE`. `go.mod`, `pyproject.toml`, and `uv.lock` pin these exact tags;
the conformance manifest names producers and consumers. A release changing protocol source publishes source, generated
bindings, SDKs, then Straw in that order. Checksums, SBOMs, attestations, provenance, and signatures for Straw binaries
and images are attached by the protected release workflow.

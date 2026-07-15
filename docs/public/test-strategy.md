# Verification strategy

Coverage is diagnostic; release confidence comes from matching each risk to the layer capable of proving it.

| Claim/risk | Evidence | Local command | Remote gate |
| --- | --- | --- | --- |
| Validation, state, errors | Go/Python unit and table tests | `make test test-python` | CI |
| Concurrency/cancellation | Race-enabled package suite | `make race` | scheduled/release |
| Bounded parser/state robustness | config/snapshot, request, DNS/URL/prefix, envelope/frame, S3 XML, and receipt-record fuzz targets | `make fuzz-smoke` | scheduled fuzz workflow |
| Protocol compatibility | Versioned manifest and cross-repository consumers | `make conformance` | compatibility |
| Public dependency graph | Empty private-module settings and exact tags | `make clean-room-check` | CI |
| Package direction | `go list` direct-import allowlist and pin assertions | `make dependency-check` | CI |
| Secret residue | Tracked-tree credential pattern scan | `make security-check` | CI |
| Default/admin/receipt deployment | namespaced isolated Compose startup, request, profile operation, and teardown | `make profile-smoke PROFILE=default\|admin\|receipts` | scheduled/release |
| HA deployment | two-worker scale check, Control loss, Redis degradation/recovery, and graceful worker loss in a namespaced stack | `make ha-smoke` | scheduled/release |
| Maintained examples | live curl, CLI, Go, and Python calls against a namespaced stack | `make examples-live` | scheduled/release |
| Documentation contracts | Docusaurus build, links, and source-surface coverage | `make docs-website` | CI |
| Supply chain | Go/npm license inventory, npm audit, govulncheck, CodeQL, dependency review, intended image-content assertions, and OCI scan | `make license-check npm-audit image-content-check` plus security workflow | Security |
| Published artifacts | checksum/attestation/signature plus pull-by-digest request smoke | release procedure | release workflow |

Preserve logs, Compose state summaries, and sanitized failure artifacts. Never preserve request URLs, headers, bodies,
tokens, or signed receipt URLs. Fuzz targets must be bounded and seeded by conformance fixtures or prior failures;
parsers, URLs/headers, DNS, envelopes, object metadata, receipt records, and frame sequences are priority surfaces.

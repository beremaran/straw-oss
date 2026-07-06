# Handoff

Task: `docs/tasks/p1/01-http-proxy-ingress-spec.md`

## Changed

- Created `docs/planning/b-http-proxy-ingress.md` to specify the P1 HTTP forward proxy contract: proxy auth, request
  mapping, raw socket error rendering, response streaming/backpressure, trailer handling, and implementation test rows.
- Updated `docs/tasks/p1/01-http-proxy-ingress-spec.md` and `docs/tasks/p1.md` to mark the task done after
  independent verification.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| The spec removes the proxy-authentication and raw-response ambiguity called out by the audit. | VERIFIED | `docs/planning/b-http-proxy-ingress.md:22`, `docs/planning/b-http-proxy-ingress.md:35`, `docs/planning/b-http-proxy-ingress.md:70`, `docs/planning/b-http-proxy-ingress.md:83` | Documentation/spec review by independent verifier |
| The spec names all implementation tasks that will consume it. | VERIFIED | `docs/planning/b-http-proxy-ingress.md:3`, `docs/planning/b-http-proxy-ingress.md:119` | Documentation/spec review by independent verifier |
| No production code is changed. | VERIFIED | Diff only adds `docs/planning/b-http-proxy-ingress.md` and updates task/handoff docs | Documentation/spec review by independent verifier |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P1 adds HTTP forward proxy | implemented | `docs/planning/b-http-proxy-ingress.md:12` |
| P1 deployment port `8081` for HTTP forward proxy | implemented | `docs/planning/b-http-proxy-ingress.md:14` |
| Proxy listener rejects CONNECT; CONNECT belongs to P1 raw tunnel work | implemented | `docs/planning/b-http-proxy-ingress.md:17` |
| API key auth carrier for proxy requests | implemented | `docs/planning/b-http-proxy-ingress.md:22` |
| `Proxy-Authorization` is never forwarded outbound | implemented | `docs/planning/b-http-proxy-ingress.md:32` |
| Proxy request maps into decoded internal request model | implemented | `docs/planning/b-http-proxy-ingress.md:38` |
| `ingress_type=http_proxy` route input | implemented | `docs/planning/b-http-proxy-ingress.md:49` |
| REST validation rules reused where applicable for proxy requests | implemented | `docs/planning/b-http-proxy-ingress.md:56` |
| Upstream 3xx/4xx/5xx remain normal upstream responses | implemented | `docs/planning/b-http-proxy-ingress.md:70` |
| Pre-header Straw errors render canonical public errors without REST success envelope | implemented | `docs/planning/b-http-proxy-ingress.md:74` |
| Post-header upstream/internal failure behavior | implemented | `docs/planning/b-http-proxy-ingress.md:83` |
| Control-to-client streaming and bounded backpressure | implemented | `docs/planning/b-http-proxy-ingress.md:87` |
| Timeout hierarchy applies to proxy transport | implemented | `docs/planning/b-http-proxy-ingress.md:99` |
| Trailer forwarding/drop/metadata-capture rules | implemented | `docs/planning/b-http-proxy-ingress.md:102` |
| Minimum implementation test rows before coding starts | implemented | `docs/planning/b-http-proxy-ingress.md:115` |
| REST streaming response format remains open | out of scope | `docs/planning/32-open-decisions.md`, owned by `docs/tasks/p1/06-rest-streaming-endpoint.md` |
| HTTP/2 downstream proxy semantics | out of scope | Owned by `docs/tasks/p2/14-http2-semantics-spec.md` |

## Verification

```sh
make check
```

Result: passed.

- Postgres-backed tests: not exercised; diff does not touch Postgres surfaces.
- Live compose verification: skipped; this is a documentation-only spec task and does not touch runtime request paths.

## Reviewer Start Points

- `docs/planning/b-http-proxy-ingress.md`
- `docs/tasks/p1/01-http-proxy-ingress-spec.md`

## Remaining Work

- None.

## Blockers

- None.

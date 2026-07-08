# Handoff

Task: `docs/tasks/p1/29-python-client-sdk.md`

## Changed

- Add: `python/straw/__init__.py` — public re-exports of the Python SDK surface.
- Add: `python/straw/client.py` — `Client`, `Request`/`Header`/`RequestBody`/`RoutingHints`, `Response`/`ResponseBody`/`Timing`,
  `ErrorResponse`/`APIError`, `Stream`/`StreamFrame`/`StreamMetadata`/`StreamTrailers`/`StreamEnd`, and the `FRAME_*` type
  constants. Stdlib-only (`urllib.request`, `json`, `struct`, `dataclasses`) — no new runtime dependency.
- Add: `python/tests/test_client.py` — 10 unittest cases against a real `http.server.HTTPServer` (no HTTP mocking).
- Add: `python/README.md` — usage guide (client construction, blocking `do`, error handling, streaming `do_stream`).
- Add: `python/pyproject.toml` — minimal setuptools packaging metadata (`straw-sdk`, no dependencies, no publish config).

Python package path: `python/straw`. Public API shape mirrors the Go SDK's field names in snake_case (e.g.
`value_base64`, `data_base64`, `request_id`). Errors surface as a `straw.APIError` exception carrying `http_status`
and a parsed `ErrorResponse` (`category`, `code`, `message`, `retryable`, `request_id`, `timeout_type`,
`retry_after_ms`, `details`). Streaming returns a `Stream` object (iterable, `__next__`/`StopIteration`) that reads
exactly one 5-byte header + payload per step from the live `http.client` response object — no `read()` with no size
bound anywhere in the path, so body frames are yielded before the terminal frame without buffering the full response.

Python test command used: `python3 -m unittest discover python/tests` (10/10 pass, ~3s).

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (fresh sub-agent, given only the task file and the diff).

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Submits `POST /api/v1/requests` with JSON envelope, Bearer auth, parses success response | VERIFIED | `python/straw/client.py` `Client.do` (~341-359), `_headers` sets `Authorization: Bearer` (~333-339) | `test_encodes_request_and_defaults_replayable` |
| Canonical error parsing exposes `category`, `code`, `retryable`, `request_id`, HTTP status | VERIFIED | `ErrorResponse`/`APIError` dataclasses (~176-206), `_api_error_from_http_error` (~381-384) | `test_parses_canonical_error_response` |
| `GET`/`HEAD`/`OPTIONS` default `replayable=True` before submission; other methods do not | VERIFIED | `_REPLAYABLE_DEFAULT_METHODS` (~26), `Request.apply_replayable_default` (~97-99), called in `do`/`do_stream` before encoding | `test_replayable_defaults_only_safe_methods`, asserted inline in `test_encodes_request_and_defaults_replayable` |
| Streaming parses all 5 documented frame types, yields body before terminal frame, no full-response buffering | VERIFIED | `_decode_frame` (~253-270), `_read_exact`/`Stream.__next__` bounded `fp.read(n)` calls (~273-322) | `test_parses_documented_frames`, `test_stream_yields_frames_incrementally_without_full_buffering` (timing-gapped frames prove incremental delivery) |
| `rg` finds no `internal/`-package references under `python/` | VERIFIED | n/a (absence proven by search) | `rg -n "github.com/beremaran/straw/v2/internal|\.\./internal|internal/" python` → no matches |
| Tasks 09 and 28 still do not claim Python SDK ownership | VERIFIED | n/a | `rg -n "Python|python" docs/tasks/p1/09-go-sdk.md docs/tasks/p1/28-sdk-cli-rest-streaming-client.md` → no matches |
| Out-of-scope items absent (no retry orchestration, no Egress/BodyRef/MITM/telemetry/CLI clients, no PyPI/release automation) | VERIFIED | `find python -type f` shows only the 5 files listed above; `pyproject.toml` has no publish config, `dependencies = []` | Directory listing + file read by verifier |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P1 SDK surface scope (`docs/planning/02-phase-boundaries.md`) | implemented | Python client added as P1 SDK surface, matches "SDK, CLI, and minimal UI surfaces" line |
| REST request schema / success envelope (`docs/planning/07-public-api-surface.md`) | implemented | `Request.to_json`/`Response.from_json` mirror documented fields | `python/straw/client.py` |
| REST streaming variant, frame types 1-5, framing format (`docs/planning/07-public-api-surface.md`) | implemented | `_decode_frame`, `Stream.__next__` | `python/straw/client.py` |
| Canonical ErrorResponse fields (`docs/planning/14-canonical-error-registry.md`) | implemented | `ErrorResponse` dataclass matches `category`/`code`/`message`/`retryable`/`request_id`/`timeout_type`/`retry_after_ms`/`details` | `python/straw/client.py` |
| Client-visible HTTP semantics — outer status vs. envelope status (`docs/planning/15-http-semantics.md`) | implemented | `Response.status` documented as upstream status in module docstring and README | `python/straw/client.py`, `python/README.md` |
| P1 SDK minimal-surface ordering (`docs/planning/31-implementation-order.md`) | already existed / not applicable | Python SDK added as this task's sole deliverable; no other P1 SDK work required |

## Verification

```sh
python3 -m unittest discover python/tests
go test ./sdk
make check
```

Result:

- Python tests: 10/10 pass.
- `go test ./sdk`: pass, unaffected (no Go files changed by this task).
- `make check`: `make fmt-check` and `make lint` pass. `make test` (`go test ./...`) has **two failures unrelated to
  this task** — confirmed by stashing this diff (`python/` only, zero `.go` changes) and re-running on a clean
  tree, where both failures reproduce identically:
  - `TestGrafanaProvisioningMatchesComposeMounts` (`deploy/observability`) fails deterministically. Root-caused via
    sub-agent: commit `aba1602a` ("chore: stuff") changed `docker-compose.yml:192` and
    `deploy/observability/grafana/provisioning/dashboards/straw.yml:11` from
    `/etc/grafana/provisioning/dashboards/straw` to `/etc/grafana/dashboards/straw` consistently, but never updated
    this test's expected-string list. This is a regression against task `docs/tasks/p1/13-observability-dashboards.md`
    (status `done`), which already owns this test and this compose mount (see
    `docs/agents/handoffs/p1-13-observability-dashboards.md:20`). Not an unowned gap — flagging here per Gap
    Ownership so it doesn't go unnoticed; the fix is a one-line revert of the two paths back to
    `/etc/grafana/provisioning/dashboards/straw`, out of scope for this Python SDK task.
  - `TestMITMHTTP2StreamCancelIsIsolated` (`internal/control`) is flaky under full-suite parallelism — failed once,
    passed on immediate re-run both standalone and as part of the full suite. Also reproduces on the pre-diff tree,
    so unrelated to this task.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped — this task adds an HTTP client library with no Control/Egress runtime changes;
  the Go SDK's existing httptest-based tests already prove the wire contract this Python client speaks against.

## Reviewer Start Points

- `python/straw/client.py`
- `python/tests/test_client.py`

## Remaining Work

- None owned by this task. Pre-existing, unrelated `TestGrafanaProvisioningMatchesComposeMounts` regression is
  owned by `docs/tasks/p1/13-observability-dashboards.md` (see Verification section above) — not created as a new
  task since an owning task already exists and is simply out of sync with a later unrelated commit.

## Blockers

- None.

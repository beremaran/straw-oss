# Handoff

Task: `docs/tasks/p2/30-ingress-http2-upload-flow-control-and-live-proof.md`

## Changed

- `internal/control/mitm_handler.go`, `internal/control/request.go`: MITM HTTP/2 requests now carry a live body reader into dispatch instead of buffering the full upload before assignment.
- `internal/control/dispatcher.go`, `internal/control/tunnel_dispatcher.go`: decoded request-body streaming now reserves NATS upload credit before reading from the client body and replenishes from e2c `CreditFrame{UploadCreditBytes}`.
- `internal/control/dispatcher_test.go`: added the focused upload-credit backpressure test.
- `deploy/docker/control.json`, `deploy/docker/egress.json`, `docker-compose.yml`, `deploy/docker/kms-mock.py`, `scripts/mitm-h2-request.go`, `deploy/docker/README.md`: local compose now exposes MITM h2 on `8083`, uses dev-only CA material plus a local AWS-KMS-compatible mock, declares the dev worker eligible for `mitm`, and documents/drives the live h2 MITM proof.
- `internal/control/mitm_leaf_bundle_aws_kms.go`: added `STRAW_MITM_LEAF_KMS_ENDPOINT` so the existing `aws-kms` envelope path can target the local compose mock.
- `.gitignore`: ignored generated `.dev/` CA material.

## Acceptance Criteria Verdicts

Filled from independent verifier reports plus the live proof recorded below.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| NATS upload-credit exhaustion applies ingress HTTP/2 backpressure without unbounded buffering. | VERIFIED | `internal/control/mitm_handler.go:177`, `internal/control/dispatcher.go:764`, `internal/control/dispatcher.go:787`, `internal/control/tunnel_dispatcher.go:414` | `TestDispatcherStreamingRequestBodyWaitsForUploadCreditBeforeReading`; `make check` |
| A live HTTP/2 MITM request succeeds through the compose stack via the normal Control -> NATS -> Egress stream protocol, with commands and result recorded. | VERIFIED | `deploy/docker/control.json:10`, `docker-compose.yml:67`, `deploy/docker/egress.json:15`, `scripts/mitm-h2-request.go:65` | Live compose command below returned `proto=HTTP/2.0 status=405` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/12-nats-protocol.md`: Control must stop or slow client upload reads when upload credit reaches zero. | implemented | `internal/control/dispatcher.go:796`, `internal/control/tunnel_dispatcher.go:414`, `internal/control/dispatcher_test.go:474` |
| `docs/planning/12-nats-protocol.md`: Egress grants additional upload credit to Control over e2c `CreditFrame`. | already existed / integrated | `internal/egress/loop.go:565`; Control consumes it at `internal/control/dispatcher.go:1173` and `internal/control/dispatcher.go:1322` |
| `docs/planning/15-http-semantics.md`: HTTP/2 flow-control interaction with NATS credit is a P2 prerequisite. | implemented | `internal/control/mitm_handler.go:177`, `internal/control/dispatcher.go:787` |
| `docs/planning/17-mitm-design-p2.md`: MITM uses server-side TLS after authenticated CONNECT and decoded dispatch. | already existed | `internal/control/mitm_connect_handler.go:94`, `internal/control/mitm_handler.go:69` |
| `docs/planning/c-http2-semantics.md` Section 4 inbound flow control: Control forwards client h2 DATA as c2e `DataFrame`s. | implemented | `internal/control/dispatcher.go:801`, `internal/control/dispatcher.go:804` |
| `docs/planning/c-http2-semantics.md` Section 4 inbound flow control: exhausted upload credit withholds client reads/window updates. | implemented | `internal/control/dispatcher.go:796`, `internal/control/dispatcher_test.go:511` |
| `docs/planning/c-http2-semantics.md` Section 4 inbound flow control: e2c upload credit allows upload to resume. | implemented | `internal/control/dispatcher.go:1173`, `internal/control/dispatcher.go:1322`, `internal/control/dispatcher_test.go:517` |
| `docs/planning/c-http2-semantics.md` Section 8 protocol translation: h2 MITM ingress translates through normal stream dispatch to the egress worker. | implemented | `scripts/mitm-h2-request.go:65`, live compose result below |
| `docs/planning/30-testing-matrix.md`: HTTP/2 flow-control backpressure test row. | implemented | `internal/control/dispatcher_test.go:474` |
| `docs/planning/30-testing-matrix.md`: HTTP/2 protocol translation live proof. | implemented | `deploy/docker/README.md:104`, live compose result below |

## Verification

```sh
go test ./internal/control -run 'TestDispatcherStreamingRequestBodyWaitsForUploadCreditBeforeReading|TestMITMHTTP2'
go test ./internal/control
go test ./cmd/control ./internal/control
docker compose config >/tmp/straw-compose-config.out
scripts/dev-mitm-ca.sh
STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY='sk_live_task30_local_admin_0123456789abcdef' docker compose up -d --build
curl -fsS http://localhost:9090/readyz
curl -fsS -H "Authorization: Bearer sk_live_task30_local_admin_0123456789abcdef" -H 'Content-Type: application/json' -d '{"role":"requester"}' http://localhost:8080/api/v1/config/tenants/22222222-2222-4222-8222-222222222222/api-keys
STRAW_REQUESTER_SECRET='sk_live_bz-h_odNnlTlaHQnLvlEZFfoJP6ME1bmDpvhHGBHKb4' go run ./scripts/mitm-h2-request.go
make check
```

Result:

- Focused and package tests passed.
- Compose config rendered successfully.
- Live compose verification: `proto=HTTP/2.0 status=405 body_prefix="<!doctype html><html lang=\"en\"><head><title>Example Domain</title>..."`
- `make check` passed.
- Postgres-backed tests: not exercised with `STRAW_TEST_POSTGRES_DSN`; diff does not touch Postgres or migrations.

## Reviewer Start Points

- `internal/control/dispatcher.go`
- `internal/control/dispatcher_test.go`
- `internal/control/mitm_handler.go`
- `docker-compose.yml`
- `scripts/mitm-h2-request.go`

## Remaining Work

- None.

## Blockers

- None.

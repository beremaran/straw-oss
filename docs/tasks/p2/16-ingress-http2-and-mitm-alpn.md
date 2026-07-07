# 16 - MITM HTTP/2 ALPN and Basic H2 Ingress

Status: done

## Objective

Implement the MITM HTTP/2 ALPN gate specified by task 14: Control offers `h2` on the authenticated MITM inner TLS
handshake only when `control.http2.enabled` is true and tenant routing policy permits MITM, while preserving HTTP/1.1
compatibility and proving basic HTTP/2 MITM requests route through the normal decoded MITM handler.

Full ingress HTTP/2 cancellation, NATS-credit flow-control, trailer, and connection-level fanout semantics were split
to `docs/tasks/p2/25-ingress-http2-stream-semantics.md` after independent verification found this original task too
large for one honest vertical slice.

## Required Planning Docs

- `docs/planning/15-http-semantics.md`
- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/c-http2-semantics.md`

## Prerequisites

- Task 14 completed.
- Task 02 completed if ALPN covers MITM.
- Task 04 completed if ALPN covers MITM.
- Task 20 completed if ALPN tests depend on cached MITM leaf selection.

## Out of Scope

- Do not implement outbound HTTP/2.
- Do not implement full ingress HTTP/2 stream cancellation, NATS-credit flow-control, trailer forwarding, or
  connection-level error fanout; these task 14 semantics are split across
  `docs/tasks/p2/25-ingress-http2-stream-semantics.md` (identity/cancellation/fanout),
  `docs/tasks/p2/29-ingress-http2-headers-and-trailers.md` (headers/trailers), and
  `docs/tasks/p2/30-ingress-http2-upload-flow-control-and-live-proof.md` (flow control/live proof).
- Do not change HTTP/1.1 ingress behavior.

## Expected Files

- Modify: `internal/control/mitm_connect_handler.go` and `internal/control/mitm_handler.go` for policy-gated MITM
  ALPN and h2 inner server setup.
- Modify: `cmd/control/main.go` so the built Control binary wires `control.http2.enabled` into the MITM CONNECT
  bootstrap.
- Test: `cmd/control/main_test.go` MITM ALPN and basic h2 request tests.

## Steps

- [x] Read all required planning docs.
- [x] Confirm task 14 includes MITM ALPN support.
- [x] Wire the built Control binary (`cmd/control`) so `control.http2.enabled` reaches the MITM CONNECT bootstrap.
- [x] Gate MITM `h2` ALPN on both static Control config and tenant MITM routing policy.
- [x] Configure the authenticated inner MITM TLS server for HTTP/2 only when the gate allows it.
- [x] Prove HTTP/2-disabled and tenant-policy-denied MITM handshakes negotiate HTTP/1.1.
- [x] Prove enabled and tenant-policy-allowed MITM handshakes negotiate `h2` and serve concurrent h2 requests through
      the normal decoded MITM handler.
- [x] Run focused ingress HTTP/2 tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./cmd/control -run TestConfigureMITMServerHTTP2ALPN -count=1`
- `make check`

## Acceptance Criteria

- MITM ALPN behavior is implemented only where task 14 specifies it and only when both `control.http2.enabled` and
  tenant MITM routing policy allow it.
- A basic HTTP/2 MITM request is translated through the normal decoded MITM handler path and concurrent h2 streams each
  receive a response.
- HTTP/1.1 ingress remains compatible.
- Full ingress HTTP/2 stream semantics remain owned by the task 25 / 29 / 30 split
  (`docs/tasks/p2/25-ingress-http2-stream-semantics.md`, `docs/tasks/p2/29-ingress-http2-headers-and-trailers.md`,
  `docs/tasks/p2/30-ingress-http2-upload-flow-control-and-live-proof.md`).

## Handoff Notes

- Document supported ingress modes and ALPN behavior.
- Include the independent verifier verdict that accepted the ALPN slice and rejected full stream semantics, with tasks
  25, 29, and 30 named as owners for the remaining scope.

## Stop Conditions

- Stop if task 14 excludes or does not define this behavior.
- Stop if a deferral would have no owning task file.

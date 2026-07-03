# 02 - MITM Ingress

Status: not started

## Objective

Implement decoded HTTPS MITM ingress on port 8083 using server-side TLS and the same internal request model as REST and
HTTP proxy.

## Required Planning Docs

- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/28-deployment.md`
- `docs/planning/27-security-controls.md`

## Prerequisites

- P1 task 02 completed.
- P1 task 04 completed.
- Task 01 completed.

## Out of Scope

- Do not implement certificate cache storage (task 04).
- Do not claim client JA3/JA4 spoofing.
- Do not implement HTTP/2 ALPN before task 16.

## Expected Files

- Create or modify: MITM ingress package.
- Modify: `cmd/control/main.go`
- Test: MITM ingress tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Add port 8083 listener behind static config.
- [ ] Terminate inbound TLS with a server-capable TLS stack.
- [ ] Use generated per-SNI leaf certificates from task 04.
- [ ] Decode HTTPS requests into the shared internal request model.
- [ ] Set routing `ingress_type=mitm`.
- [ ] Preserve destination policy, header stripping, and tenant isolation behavior.
- [ ] Add tests for TLS termination, decoded request mapping, ingress_type routing, denied destinations, and shutdown.
- [ ] Run focused MITM ingress tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused MITM ingress tests.
- `make check`

## Acceptance Criteria

- MITM HTTPS requests enter the existing dispatch pipeline as decoded requests.
- The server TLS implementation is not confused with outbound TLS fingerprinting.
- Port 8083 is mapped only when MITM is enabled.

## Handoff Notes

- Document TLS stack choice and limitations.

## Stop Conditions

- Stop before adding ALPN/HTTP2 behavior.
- Stop if MITM would mutate client TLS fingerprint claims.
- Stop if a deferral would have no owning task file.

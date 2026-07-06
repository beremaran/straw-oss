# 26 - Upstream Connection Pooling Implementation

Status: done

## Objective

Implement the optional, default-off Egress upstream connection pool specified in
`docs/planning/b-upstream-connection-pooling.md`, preserving the direct-local SSRF invariant and P0 transport defaults
when disabled.

## Context (gap being closed)

P1 task 16 specified upstream connection pooling but deliberately did not change transport code. The current Egress
executor still hard-codes P0 defaults:

- `internal/egress/executor.go:118` sets `DisableKeepAlives: true`.
- `internal/egress/executor.go:119` sets `ForceAttemptHTTP2: false`.
- `internal/egress/executor_test.go:489` asserts keep-alives stay disabled.

The new spec makes pooling allowed only behind `egress.upstream_connection_pool.enabled` and only after tests prove the
pool-key and SSRF boundaries.

## Required Planning Docs

- `docs/planning/b-upstream-connection-pooling.md`
- `docs/planning/16-egress-execution.md` ("DNS Validation and Dial Target Invariant")
- `docs/planning/27-security-controls.md` ("SSRF Enforcement by Resolution Mode")
- `docs/planning/30-testing-matrix.md` (P1 upstream connection pooling row)
- `docs/planning/24-static-configuration.md` (Egress config shape)

## Prerequisites

- P1 task 16 completed (pooling spec and test rows exist).

## Out of Scope

- Do not enable outbound HTTP/2.
- Do not add proxy-mode connection pooling; the P1 pooling spec is direct-local only.
- Do not change routing, fallback, or retry behavior.
- Do not weaken the resolver/validator/dialer invariant.

## Expected Files

- Modify: `internal/config/config.go` and config validation tests for `egress.upstream_connection_pool.*`.
- Modify: `internal/egress/executor.go` to construct disabled-default and enabled pooling transports.
- Modify: `cmd/egress/main.go` so the built Egress binary passes pooling config into the executor.
- Test: `internal/egress/executor_test.go` and config tests for disabled default, enabled reuse, tenant isolation,
  DNS rebinding/SSRF, second-resolution guard, eviction/shutdown, and stale-connection fallback.

## Steps

- [x] Read all required planning docs.
- [x] Add Egress static config fields and validation for `egress.upstream_connection_pool.*`.
- [x] Keep the disabled default identical to P0: `DisableKeepAlives=true` and outbound HTTP/2 disabled.
- [x] Implement enabled direct-local pooling keyed by tenant, resolution mode, scheme, original hostname, port,
      validated dial IP, and fingerprint profile.
- [x] Revalidate DNS and destination policy before every reuse; discard pooled connections whose dial IP is absent
      from the current validated set.
- [x] Ensure the HTTP/TLS library still dials only validated IPs and does not perform a second hostname resolution.
- [x] Implement idle timeout, max lifetime, error eviction, and worker shutdown cleanup.
- [x] Add the tests listed in Expected Files.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/config ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- Default config leaves keep-alives disabled and outbound HTTP/2 disabled, proven by existing and updated Egress tests.
- Enabled config reuses a connection only for the exact spec pool key; cross-tenant, cross-host, cross-port,
  cross-IP, and cross-fingerprint reuse is rejected by tests.
- Every request resolves and validates destination policy before reuse; a DNS rebinding test proves a stale pooled IP
  is discarded.
- The enabled transport still dials only validated IPs; a test proves no independent second hostname resolution occurs.
- Idle timeout, max lifetime, protocol/TLS/body errors, and worker shutdown close pooled connections without leaking
  goroutines.

## Handoff Notes

- Record the concrete pool-key implementation and the tests proving each Section 30 row.
- State whether live compose verification was run; if skipped, explain why.

## Stop Conditions

- Stop if the implementation would require outbound HTTP/2.
- Stop if pooling would cause a second DNS resolution after policy validation.
- Stop if a deferral would have no owning task file.

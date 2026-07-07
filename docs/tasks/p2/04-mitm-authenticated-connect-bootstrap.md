# 04 - MITM Authenticated CONNECT Bootstrap

Status: not started

## Objective

Replace the current direct TLS MITM listener with an explicit-proxy CONNECT bootstrap so Control authenticates the
tenant before starting the inner TLS handshake and leaf certificate lookup/generation. Keep decoded MITM request
dispatch working, but do not add encrypted cache storage, Redis locking, or flood controls in this task.

## Context (gap being closed)

The original P2 task 04 mixed two separate implementation surfaces: reshaping MITM ingress so tenant identity is known
before leaf generation, and adding the encrypted leaf cache. That made agents stop at the task's tenant-before-handshake
constraint instead of implementing a vertical slice.

Current Control still starts MITM as a direct TLS listener: `cmd/control/main.go:320` builds an `http.Server`,
`cmd/control/main.go:326` calls `ListenAndServeTLS`, and `cmd/control/main.go:345` installs `tls.Config.GetCertificate`
with only `hello.ServerName` available. Current MITM request authentication happens inside the HTTPS handler at
`internal/control/mitm_handler.go:19`, after the TLS certificate has already been selected. The existing raw CONNECT
path already authenticates before hijack at `internal/control/connect_handler.go:41`, validates the CONNECT authority at
`internal/control/connect_handler.go:48`, hijacks at `internal/control/connect_handler.go:62`, and has a reusable
`200 Connection Established` helper at `internal/control/connect_handler.go:148`.

Appendix C now requires tenant identity before cache keys, KMS AAD, or per-tenant flood limits are evaluated. This task
owns the runtime prerequisite: authenticate CONNECT first, then run the inner server-side TLS handshake with a
tenant-aware leaf-generation hook. The encrypted cache itself is owned by
`docs/tasks/p2/20-mitm-leaf-cert-cache.md`.

## Required Planning Docs

- `docs/planning/c-mitm-leaf-certificate-design.md` (tenant-before-leaf lookup and direct TLS replacement, lines ~13-21
  and ~83-86)
- `docs/planning/17-mitm-design-p2.md` (TLS library boundary and explicit CONNECT MITM shape, lines ~5-18)
- `docs/planning/10-routing-model.md` (`ingress_type=mitm` routing condition, lines ~13-24)
- `docs/planning/27-security-controls.md` (Proxy-Authorization, header stripping, SNI vs Host mismatch, and CONNECT
  target normalization, lines ~30-41 and ~119-136)
- `docs/planning/28-deployment.md` (MITM port purpose, lines ~13-25)

## Prerequisites

- Task 02 completed (decoded MITM handler and `ingress_type=mitm` dispatch path exist).
- Task 03 completed (operator-provided MITM CA config exists).
- Task 19 completed (leaf-bundle KMS provider exists for the follow-up cache task).
- P1 task 05 completed (raw CONNECT auth, target validation, hijack, and tunnel helpers exist).

## Out of Scope

- Do not implement encrypted leaf cache storage, Redis keys/locks, local singleflight, or flood controls; task 20 owns
  those.
- Do not change the private-key storage policy chosen in task 01.
- Do not implement MITM HTTP/2 ALPN; task 16 owns that.
- Do not implement tenant-admin CA configure/rotate APIs; task 18 owns those.

## Expected Files

- Modify: `cmd/control/main.go` for `cmd/control` to construct a MITM CONNECT listener instead of the direct TLS
  listener.
- Modify or add: `internal/control/mitm_handler.go` / `internal/control/mitm_connect_handler.go` for authenticated
  CONNECT bootstrap and inner TLS request serving.
- Modify if useful: `internal/control/connect_handler.go` to reuse CONNECT authentication, target validation, hijack,
  and `200 Connection Established` helpers without duplicating logic.
- Test: `internal/control/mitm_handler_test.go` and `cmd/control/main_test.go`.

## Steps

- [ ] Read all required planning docs.
- [ ] Add a focused failing test proving MITM CONNECT authenticates the proxy request before inner TLS certificate
      lookup.
- [ ] Reuse or extract the existing CONNECT authentication, target validation, hijack, and `200 Connection Established`
      helpers for the MITM listener.
- [ ] Start an inner `tls.Server` on the hijacked CONNECT connection and call a leaf lookup/generation hook with the
      authenticated tenant identity and normalized SNI/CONNECT authority.
- [ ] Serve the decoded HTTPS request through the MITM dispatch path using the CONNECT-authenticated identity, without
      requiring `Proxy-Authorization` inside the inner HTTPS request.
- [ ] Remove or fail closed the old direct TLS MITM `GetCertificate` path so it cannot become a cache-writing path with
      no tenant identity.
- [ ] Preserve Host/SNI mismatch rejection, destination policy behavior, header stripping, shutdown, and
      `ingress_type=mitm`.
- [ ] Add tests for CONNECT auth failure, CONNECT target validation, tenant identity reaching the cert hook, decoded
      MITM dispatch after inner TLS, and direct TLS path removal/fail-closed behavior.
- [ ] Run focused MITM bootstrap tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/control -run 'TestMITM|TestConnect'`
- `make check`

## Acceptance Criteria

- MITM port 8083 accepts explicit-proxy CONNECT, authenticates the CONNECT request, writes `200 Connection Established`,
  and then serves an inner HTTPS request through the decoded MITM dispatch path.
- The leaf lookup/generation hook receives authenticated tenant identity plus normalized SNI/CONNECT authority before
  any certificate is selected, proven by a focused test.
- Inner HTTPS requests do not need to carry `Proxy-Authorization`; they use the identity authenticated on CONNECT.
- The old direct TLS `GetCertificate` MITM path is removed or fails closed and cannot write tenant-scoped cache entries
  without tenant identity.
- No encrypted leaf cache, Redis lock, singleflight, or flood-control behavior is implemented in this task; those remain
  owned by `docs/tasks/p2/20-mitm-leaf-cert-cache.md`.

## Handoff Notes

- Document the runtime flow from CONNECT auth to inner TLS to decoded MITM dispatch.
- Document the leaf lookup hook signature and exactly which tenant/SNI/authority values task 20 should use.
- Document whether the old direct TLS path was removed or made fail closed.

## Stop Conditions

- Stop if implementing authenticated CONNECT bootstrap requires changing the resolved tenant-scoped leaf storage
  policy.
- Stop if the smallest working bootstrap would require adding a new dependency.
- Stop if MITM decoded dispatch cannot use the CONNECT-authenticated identity without crossing into unrelated auth
  refactors.
- Stop if a deferral would have no owning task file.

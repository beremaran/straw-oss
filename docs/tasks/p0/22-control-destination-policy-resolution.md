# 22 - Control Destination Policy Resolution

Status: done

## Objective

Add the Control-side per-request policy resolver that validates target destinations, deny rules, header injection, and
fingerprint profile support before task 24 dispatches a request to Egress.

## Required Planning Docs

- `docs/planning/27-security-controls.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 09 completed.
- Task 19 completed.

## Out of Scope

- Do not wire the resolver into the REST request handler (task 24).
- Do not implement MITM or proxy ingress.
- Do not implement redirect following.
- Do not move Egress resolved-IP validation into Control; Egress still performs final DNS/IP policy validation.

## Expected Files

- Create or modify: `internal/control` destination policy resolver.
- Test: focused resolver tests.

## Steps

- [x] Read all required planning docs.
- [x] Define a resolver API that consumes the validated request, captured tenant snapshot, selected route/executor, and
      worker capabilities, and returns the `strawpb.DestinationPolicy` plus resolved injection and fingerprint data.
      (Fingerprint validation uses `config.FingerprintProfile.SupportedByWorker` from the snapshot, since P0's
      `RegisterRequest` wire protocol carries no per-worker fingerprint capability list to consume directly.)
- [x] Reject URL userinfo before dispatch and ensure public errors/details never contain unsanitized full URLs,
      credentials, worker IDs, session IDs, or NATS subjects.
- [x] Evaluate deny rules with lowercase hostnames, trailing-dot normalization, default ports, IPv4/IPv6 literal
      handling, CNAME semantics, metadata IPs, and private/link-local defaults. IDNA/punycode was completed by
      `docs/tasks/p1/21-idna-hostname-support.md`.
- [x] Resolve ordered header-injection operations, enforcing the Section 15 denied-header table, sensitive-header role
      restrictions, duplicate `set` rejection, size bounds, and CR/LF rejection.
- [x] Validate the requested or tenant-default fingerprint profile against the selected worker's supported profiles and
      map unsupported profiles to `unsupported_fingerprint`.
- [x] Produce direct-local or upstream-proxy destination resolution mode according to deployment config and reject
      untrusted upstream-proxy remote resolution. (P0 has no upstream-proxy config surface anywhere in this repo, so
      real callers always resolve to direct-local; the upstream-proxy-remote path is implemented and tested but
      unreachable from any current caller.)
- [x] Map resolver failures through the existing error registry codes (`destination_denied`, `header_injection_failed`,
      `unsupported_fingerprint`, `invalid_request`) without leaking internals.
- [x] Add tests for deny normalization, metadata IP defaults, allow overrides, CNAME denial, injection safety,
      fingerprint mismatch, upstream-proxy trust policy, and public-safe error details.
- [x] Run focused destination-policy resolver tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused destination-policy resolver tests.
- `go test ./internal/control`
- `make check`

## Acceptance Criteria

- Control can produce the destination-policy bundle consumed by `internal/egress/executor.go`.
- Deny rules and injection policies are evaluated from the immutable tenant snapshot captured at request start.
- Public errors use canonical registry codes and do not leak internal topology or secrets.
- Egress remains responsible for final resolved-IP validation immediately before connect.

## Handoff Notes

- Document the resolver API and which snapshot fields it consumes.
- Note that request-handler wiring is deferred to `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.

## Stop Conditions

- Stop before adding redirect following.
- Stop if a policy rule would conflict with the Section 27 SSRF invariant.
- Stop if a deferral would have no owning task file.

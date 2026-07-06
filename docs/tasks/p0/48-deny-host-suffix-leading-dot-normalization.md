# 48 - Deny `host_suffix` Leading-Dot Normalization Fix

Status: done

## Objective

A `host_suffix` deny rule created with a leading-dot value (e.g. `.evil.example`) enforces exactly like the same
rule without the leading dot: it denies the suffix host and all its subdomains, both in Control's pre-dispatch
check and in Egress's post-resolution check. After this task, no accepted `host_suffix` (or `host`) deny rule can
be silently non-enforcing because of a leading dot in its value.

## Context (gap being closed)

The 2026-07-07 live compose verification (`docs/tasks/p0/46-live-compose-verification.md`, surface 43) found that a
`host_suffix` deny rule created via the admin API with a leading-dot value is accepted with HTTP 200 but **never
matches** — a request to a subdomain of that suffix is not denied and instead falls through to Egress (where it
failed DNS in the test). A tenant admin who writes `.evil.example` believes they denied that suffix; they did not.

Current-code evidence:

- `internal/control/config_admin_handlers.go:695-696` — `normalizeHostname` does
  `strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")`: it trims a *trailing* dot only, so a
  leading-dot value like `.evil.example` is stored verbatim in `deny_rules.normalized_host`.
- `internal/control/destination_policy.go:354-364` — `hostMatchesSuffix(host, suffix)` returns
  `host == suffix || strings.HasSuffix(host, "."+suffix)`. With `suffix = ".evil.example"` the second arm searches
  for `"..evil.example"` (double dot), which no real host ends with, so the rule never matches.
- Live evidence (2026-07-07): `{"type":"host_suffix","value":".blocked.example","action":"deny"}` → 200, then
  `GET https://sub.blocked.example/` was **not** denied; the identical rule with value `blocked2.example` (no
  leading dot) → `GET https://sub.blocked2.example/` returned `destination_denied`.
- `internal/egress/executor.go` `hostMatchesSuffix` mirrors the Control matcher (per the comment at
  `internal/control/destination_policy.go:351-353`), so the leading-dot value is non-enforcing on the Egress side
  too — the rule is fully inert, not merely a Control pre-check miss.

The gap exists because task 43 (deny-rule taxonomy alignment) formalized `host_suffix` and its normalization but
neither stripped a leading dot on write nor rejected it, and both matchers assume the stored suffix has no leading
dot.

## Required Planning Docs

- `docs/planning/27-security-controls.md` — "Destination Deny Normalization and CIDR Defaults" (section at
  line ~40): how host/host_suffix values are normalized and matched.
- `docs/planning/26-config-management-api-surface.md` — Deny Rule schema and the `host_suffix` type semantics.

## Prerequisites

- Task 43 done (built the `host_suffix` type and `normalizeHostname`; this task fixes its normalization). Done.

## Out of Scope

- Do not change `cidr` / `metadata_ip` / `private_range` CIDR normalization, or `cname_suffix` semantics; this task
  is scoped to hostname (`host`, `host_suffix`) leading-dot handling only.
- Do not change the wire/DB column shape of `deny_rules`; this is a value-normalization fix, not a schema change.
- Do not weaken the existing trailing-dot trimming or lowercase/trim behavior.

## Expected Files

- Modify: `internal/control/config_admin_handlers.go` (`normalizeHostname`: also strip a single leading `.` so
  `.evil.example` and `evil.example` normalize identically — or reject a leading dot at validation; pick one and
  state which in the handoff).
- Verify/keep: `internal/control/destination_policy.go` (`hostMatchesSuffix`) and `internal/egress/executor.go`
  (`hostMatchesSuffix`) stay in agreement with the chosen normalization.
- Test: `internal/control/config_admin_handlers_test.go` and/or `internal/control/destination_policy_test.go`
  (leading-dot `host_suffix` matches subdomains and the exact host; parity with the no-leading-dot form).

## Steps

- [x] Read the required planning doc sections listed above.
- [x] Decide and implement: strip a single leading `.` in `normalizeHostname` (so both forms store `evil.example`),
      OR reject a leading-dot value at deny-rule validation with a 400. Record which and why in the handoff. (Strip
      is preferred: a leading dot is a natural way to write a suffix and silently accepting-but-not-enforcing is the
      failure mode being closed.) — Chose strip; dot-only values (normalize to empty) are rejected with 400.
- [x] Confirm `hostMatchesSuffix` in both `internal/control/destination_policy.go` and
      `internal/egress/executor.go` matches subdomains and the exact suffix host under the chosen normalization.
- [x] Add a unit test asserting a `host_suffix` rule created with a leading-dot value denies both the exact host
      and a subdomain, and behaves identically to the no-leading-dot form.
- [x] Run focused tests (`go test ./internal/control ./internal/egress`), then `make check`.
- [x] If the compose stack is available, re-run the surface-43 live check: create `.blocked.example` `host_suffix`
      deny and confirm `sub.blocked.example` is `destination_denied`. — Ran on the rebuilt stack: denied, and the
      exact host `blocked.example` is denied too.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./internal/egress`
- `make check`

## Acceptance Criteria

- A `host_suffix` deny rule created with value `.evil.example` denies both `evil.example` and `x.evil.example`,
  proven by a unit test that fails against the current `normalizeHostname`.
- The leading-dot and no-leading-dot forms of the same `host_suffix` value produce identical enforcement, proven by
  the same or an adjacent test.
- `grep` of the deny-rule matchers shows no `host_suffix` code path that can accept a value it will never match
  (either the value is normalized to a matchable form, or such a value is rejected with 400 at write time).
- If the fix is "reject", the API returns a 400 with a reason for a leading-dot value, proven by a handler test; if
  the fix is "strip", the stored `normalized_host` has no leading dot, proven by a store/normalization test.

## Handoff Notes

- Record the decision (strip vs reject) and the rationale.
- Record whether the live surface-43 re-check was run and its result.
- If Egress's `hostMatchesSuffix` needed any change to stay in parity, note it (this crosses into `cmd/egress`
  runtime behavior and must be verified, not assumed).

## Stop Conditions

- Stop if the planning doc specifies a leading-dot convention that contradicts the chosen normalization.
- Stop if fixing Control-side matching alone would leave Egress-side enforcement inconsistent (both must agree).
- Stop if a deferral would have no owning task file.

# 21 - IDNA Hostname Support

Status: not started

## Objective

Accept internationalized (non-ASCII) target hostnames by converting them to punycode/A-labels before destination
policy evaluation and dispatch, replacing the current fail-closed rejection.

## Context (gap being closed)

Task p0/22 (Control destination policy resolution) deliberately rejects non-ASCII hostnames fail-closed and its
handoff flagged IDNA as unowned. That is safe but wrong for legitimate international domains. This task is the owner.
Security constraint: policy evaluation (deny rules, host/CNAME suffix matching) must run on the normalized A-label
form so that Unicode look-alikes cannot bypass suffix or host rules.

## Required Planning Docs

- `docs/planning/27-security-controls.md` (destination validation)
- `docs/planning/15-http-semantics.md` (host handling)

## Prerequisites

- P0 complete (destination policy pipeline stable).

## Out of Scope

- No new dependency without the AGENTS.md stop condition: prefer `golang.org/x/net/idna` only if already in the
  module graph; otherwise stop and ask.
- No display-form (U-label) round-tripping in telemetry beyond recording the normalized form.

## Steps

- [ ] Normalize hostnames to A-labels at the Control ingress boundary (one place, before policy evaluation).
- [ ] Evaluate all deny/suffix/CNAME rules against the normalized form; add look-alike bypass tests
      (e.g. a Unicode homograph of a denied host must still be denied).
- [ ] Keep rejecting hostnames that fail IDNA conversion.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Acceptance Criteria

- A valid internationalized hostname dispatches successfully end-to-end.
- A homograph of a denied hostname is still denied (test).
- Invalid IDNA input is rejected with the existing validation error code.

## Stop Conditions

- Stop if a new dependency is needed (ask first, per AGENTS.md).
- Stop if a deferral would have no owning task file.

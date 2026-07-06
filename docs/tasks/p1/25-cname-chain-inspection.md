# 25 - CNAME Chain Inspection for Deny Rules

Status: not started

## Objective

Enforce `denied_cname_suffixes` against every hop of the target's CNAME chain, not just the final canonical name:
a deny rule whose suffix matches an intermediate (non-final) CNAME hop rejects the request with
`destination_denied`, closing the bypass where a denied intermediary hides behind a clean final target.

## Context (gap being closed)

`docs/agents/handoffs/26-egress-destination-policy-precedence.md` ("CNAME Chain Depth", lines 57-63) flagged —
with no owning task — that only the final canonical name is inspected. Verified in current code (2026-07-06
sweep):

- `internal/egress/executor.go:52-58` — the `Resolver` interface documents that Go's `net.Resolver.LookupCNAME`
  follows the whole chain internally but returns only the final name, so "only the final canonical name is
  available for denied_cname_suffixes enforcement".
- `internal/egress/executor.go:694-705` — enforcement matches suffixes against that single returned name.

`docs/planning/27-security-controls.md:49` lists "CNAME chains" in the deny-rule normalization requirements, the
same list whose IDNA row is owned by `docs/tasks/p1/21-idna-hostname-support.md`; this task is the sibling owner
for the CNAME-chain row. The p0/26 task text explicitly permitted "document why deeper chain inspection is out of
reach with the stdlib resolver" — this task lifts that ceiling. Exposing intermediate hops requires a resolver
that returns raw CNAME records (e.g. a custom `miekg/dns`-based resolver): a new dependency, which is an AGENTS.md
stop condition and this task's decision gate.

## Required Planning Docs

- `docs/planning/27-security-controls.md` ("Destination Deny Normalization and CIDR Defaults", lines ~40-55)
- `docs/planning/16-egress-execution.md` (resolution/validation order in the egress request path)

## Prerequisites

- P0 complete (egress destination policy enforcement exists; p0/26 done).

## Out of Scope

- No change to `denied_host_suffixes`, CIDR, or IP-literal enforcement semantics.
- No Control-side DNS resolution — CNAME enforcement stays in Egress, per p0/26.
- No resolver caching layer; per-request lookup semantics stay as they are.

## Expected Files

- Modify: `internal/egress/executor.go` (extend the `Resolver` boundary to expose the CNAME record chain; match
  `denied_cname_suffixes` against every hop; update the stale interface comment at lines 52-58).
- Modify: `cmd/egress` wiring only if the resolver construction changes.
- Test: an intermediate-hop CNAME matching a denied suffix is rejected with `destination_denied` /
  `cname_denied_suffix`; a chain with no matching hop still dispatches; final-hop matching keeps working.

## Steps

- [ ] Read all required planning docs.
- [ ] Stop and ask before adding any DNS dependency (e.g. `miekg/dns`); proceed only with explicit approval, or
      with a stdlib-only approach if one is found.
- [ ] Extend the `Resolver` interface to return the CNAME chain (all hops, normalized: lowercase, trailing dot
      stripped) and implement it in the default resolver (`cmd/egress` binary path).
- [ ] Match `denied_cname_suffixes` against every hop with the existing exact/dot-boundary suffix rules.
- [ ] Update the `internal/egress/executor.go:52-58` limitation comment and the handoff-26 "CNAME Chain Depth"
      section to name this task / the new behavior.
- [ ] Add the tests listed in Expected Files.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/egress`
- `make check`

## Acceptance Criteria

- A `denied_cname_suffixes` entry matching only an intermediate CNAME hop rejects the request with
  `destination_denied` and the `cname_denied_suffix` fact (proven by an executor test with a fake resolver
  returning a multi-hop chain).
- Final-hop and no-match behavior is unchanged (existing tests still pass).
- The "does not expose intermediate hops" limitation comment in `internal/egress/executor.go` is gone or rewritten
  to describe chain inspection, and handoff 26's "CNAME Chain Depth" section names this task as owner.

## Handoff Notes

- Record the dependency decision (which resolver library, or why stdlib sufficed) and the chain normalization
  applied per hop.

## Stop Conditions

- Stop before adding any new dependency — the raw-chain resolver needs user approval (AGENTS.md).
- Stop if resolver behavior differences (search domains, DNSSEC, split-horizon) make chain results ambiguous for
  the tests — ask.
- Stop if a deferral would have no owning task file.

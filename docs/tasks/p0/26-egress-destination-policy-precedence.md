# 26 - Egress Destination Policy Precedence and Suffix Enforcement

Status: done

## Objective

Fix `internal/egress/executor.go`'s destination-policy validation so it matches the semantics Control's
destination-policy resolver (task 22) actually promises, and wire the two `DestinationPolicy` bundle fields Egress
currently never reads.

This task exists because of two gaps identified while implementing task 22
(`docs/agents/handoffs/22-control-destination-policy-resolution.md`):

1. **AllowedCidrs is not a true override.** `internal/egress/executor.go`'s `validateCIDRPolicy`/`deniedByDefault`
   (originally task 11) treats `DestinationPolicy.allowed_cidrs` as a restrict-to-allowlist gate: if non-empty, an
   address must be inside it, but the address is *still* separately checked against the private/loopback/link-local/
   metadata booleans and the static default-deny prefix list regardless. Control's resolver
   (`internal/control/destination_policy.go`, `evaluateLiteralIPDeny`) treats an explicit tenant allow-type deny rule
   as a genuine override, per `docs/planning/27-security-controls.md` ("Private/link-local/metadata IP blocks are
   denied by default unless a tenant admin explicitly allows them for a tenant or deployment"). Net effect today: a
   tenant that allow-lists a private/loopback/metadata IP passes Control's pre-dispatch check but is still rejected by
   Egress after DNS resolution — the "allow override" feature does not actually work end-to-end.
2. **denied_host_suffixes and denied_cname_suffixes are never read.** Control's resolver populates both fields on the
   `DestinationPolicy` bundle it sends (host-type and cname-type tenant deny rules), but
   `internal/egress/executor.go` has no code path that consults either field. cname-type deny rules can only be
   meaningfully enforced by Egress, since Control performs no DNS resolution — so as it stands, cname-type deny rules
   have no enforcement anywhere in the system.

## Required Planning Docs

- `docs/planning/27-security-controls.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 11 completed.
- Task 22 completed.

## Out of Scope

- Do not change Control's resolver semantics (`internal/control/destination_policy.go`) — it already implements the
  target behavior; this task brings Egress into agreement with it.
- Do not implement redirect following or MITM.
- Do not add a per-tenant `allow_private_ranges`/`allow_loopback`/`allow_link_local`/`allow_multicast`/
  `allow_metadata_ips` config surface — those booleans remain deployment-level and P0 has no config wiring for them;
  this task only fixes how `allowed_cidrs` and the suffix fields interact with the existing checks.

## Expected Files

- Modify: `internal/egress/executor.go`
- Test: focused executor destination-policy tests.

## Steps

- [x] Read all required planning docs. Also read `internal/control/destination_policy.go`'s `evaluateLiteralIPDeny`
      doc comment for the exact override semantics to match.
- [x] Change `validateCIDRPolicy`/`deniedByDefault` (or their replacement) so an address matching
      `DestinationPolicy.allowed_cidrs` short-circuits as allowed, taking precedence over the private/loopback/
      link-local/metadata booleans and the static default-deny prefix list. `Is4In6` (IPv4-mapped IPv6) remains
      unconditionally denied regardless of `allowed_cidrs` — do not let it be overridden.
- [x] Preserve existing behavior when `allowed_cidrs` is empty or does not match: default-deny booleans, static
      prefix list, and `denied_cidrs` still apply exactly as today.
- [x] Wire `denied_host_suffixes`: reject the request if the request's Host (already available to Egress from
      `RequestStart`) has any entry in `denied_host_suffixes` as an exact match or dot-boundary suffix, mapping to
      `destination_denied` via the existing `dnsDeniedIPFact`-style fact-to-code path (or an equivalent host-deny
      fact if a distinct one is warranted — check the Section 16 fact table before adding a new fact/code pair).
- [x] Wire `denied_cname_suffixes`: after DNS resolution, if the resolver returns CNAME records for the target host,
      reject the request if any CNAME in the chain has a `denied_cname_suffixes` entry as an exact match or
      dot-boundary suffix. Go's standard resolver (`net.Resolver`) does not expose the CNAME chain through
      `LookupIPAddr`; use `LookupCNAME` (single hop) or document why deeper chain inspection is out of reach with the
      stdlib resolver alone, rather than silently skipping multi-hop chains.
- [x] Add tests for: allowed_cidrs overriding a private/loopback/metadata address, allowed_cidrs not overriding
      Is4In6, denied_host_suffixes rejection, denied_cname_suffixes rejection, and confirmation that existing
      default-deny/denied_cidrs behavior is unchanged when allowed_cidrs is absent.
- [x] Run focused executor destination-policy tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused executor destination-policy tests.
- `go test ./internal/egress`
- `make check`

## Acceptance Criteria

- An address covered by `DestinationPolicy.allowed_cidrs` is dialed even when it is also private/loopback/link-local/
  metadata/in the static default-deny list (except `Is4In6`, which stays always denied).
- A request whose resolved host or CNAME chain matches `denied_host_suffixes`/`denied_cname_suffixes` is rejected
  before connect.
- Existing default-deny and `denied_cidrs` behavior is unchanged for requests with no `allowed_cidrs` entries.
- The dial-target invariant (`docs/planning/16`: resolver, validator, and dialer are one unit) still holds — this
  task does not introduce a second, independent resolution path.

## Handoff Notes

- Document the exact precedence order implemented (allowed_cidrs override -> denied_cidrs -> metadata -> private/
  loopback/link-local/multicast -> default-deny prefixes -> host/cname suffixes).
- Note the CNAME chain depth actually inspected (single-hop via `LookupCNAME` unless a deeper approach was
  implemented) so a future task can extend it if needed.

## Stop Conditions

- Stop if fixing the precedence would require deviating from the dial-target invariant.
- Stop if the stdlib resolver cannot support the required CNAME inspection without a new dependency — report back
  rather than adding one silently.
- Stop if a deferral would have no owning task file.

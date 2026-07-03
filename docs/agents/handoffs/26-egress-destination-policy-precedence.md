# Handoff

Task: `docs/tasks/p0/26-egress-destination-policy-precedence.md`

## Changed

- `internal/egress/executor.go`:
  - `validateResolvedIP` now implements the precedence order
    `Is4In6 (unconditional) -> allowed_cidrs override -> denied_cidrs -> metadata/private/loopback/link-local/
    multicast booleans -> default-deny prefixes`, matching
    `internal/control/destination_policy.go`'s `evaluateLiteralIPDeny`. `allowed_cidrs` is now a true override: a
    match short-circuits every later check, including `denied_cidrs`. The old restrict-to-allowlist gate (deny if
    `allowed_cidrs` non-empty and the address doesn't match) is removed — Control never treated `allowed_cidrs` as a
    gate, only as an override list, so Egress previously disagreed with Control's own documented semantics.
    `Is4In6` is checked before `allowed_cidrs` and is never overridable, per the task's explicit requirement.
  - Added `validateHostSuffixPolicy` (exact-match or dot-boundary suffix against `denied_host_suffixes`), called in
    `Execute` right after `validateStart`, before any DNS resolution — no DNS lookup is needed to reject on the
    literal request Host.
  - Added `validateCNAMESuffixPolicy`, called in `dialValidated` after resolved-IP validation succeeds and before
    dialing. Only issues a `LookupCNAME` call when `denied_cname_suffixes` is non-empty, to avoid an extra DNS
    round-trip when the tenant has no cname deny rules.
  - Extended the `Resolver` interface with `LookupCNAME(ctx, host) (string, error)`; `defaultResolver` implements it
    via `net.DefaultResolver.LookupCNAME`.
  - New facts: `host_denied_suffix`, `cname_denied_suffix`, both mapped to the existing
    `ERROR_CODE_DESTINATION_DENIED` code (no new ErrorCode was needed — only the diagnostic `details["fact"]` value
    is new, consistent with the Section 16 fact table using `details["fact"]` for diagnosis).
- `internal/egress/executor_test.go`: added `TestExecutorAllowedCIDRsOverridePrivateAndMetadataDenials`,
  `TestExecutorIs4In6NeverOverriddenByAllowedCIDRs`, `TestExecutorRejectsDeniedHostSuffix`,
  `TestExecutorRejectsDeniedCNAMESuffix`; added `staticResolver.LookupCNAME` (returns the host unchanged, i.e. no
  CNAME) and a `cnameResolver` test type that returns a fixed canonical name for CNAME-suffix tests.
- `internal/control/dispatcher_test.go`: added `dispatchResolver.LookupCNAME` so its existing `Resolver` test double
  still satisfies the extended interface.

## Verification

```sh
go test ./internal/egress/... -run TestExecutor -v
make check
```

Result: all pass, including the new precedence/suffix tests; `golangci-lint run --max-issues-per-linter 0
--max-same-issues 0` reports 0 issues.

## Reviewer Start Points

- `internal/egress/executor.go` — `validateResolvedIP`, `validateHostSuffixPolicy`, `validateCNAMESuffixPolicy`,
  `dialValidated`, `Resolver` interface.
- `internal/egress/executor_test.go` — the four new tests listed above.

## Precedence Order Implemented

`Is4In6 (unconditional deny)` -> `allowed_cidrs override (allow)` -> `denied_cidrs (deny)` -> `metadata/private/
loopback/link-local/multicast deployment-level booleans (deny unless allowed)` -> `static default-deny prefix list
(deny)` -> `denied_host_suffixes (deny, checked pre-DNS on the literal request Host)` -> `denied_cname_suffixes
(deny, checked post-DNS on the resolved canonical name)`.

## CNAME Chain Depth

Only the final canonical name is inspected, via `net.Resolver.LookupCNAME`, which follows the entire CNAME chain
internally but only returns the final target — Go's stdlib resolver does not expose intermediate hops through any
public API. A `denied_cname_suffixes` entry matching an intermediate (non-final) CNAME hop is not detected. If deeper
chain inspection is needed, it requires a resolver capable of returning the raw CNAME record chain (e.g. a custom
`miekg/dns`-based resolver), which is a new dependency and out of this task's scope.

## Remaining Work

- None. Nothing in this change is faked, stubbed, or deferred — grep of the diff for `InMemory`/`stub`/`fake`/
  `synthetic`/`TODO` had no hits.

## Blockers

- None.

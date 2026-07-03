# Handoff

Task: `docs/tasks/p0/22-control-destination-policy-resolution.md`

## Changed

- `internal/control/destination_policy.go` (new): the Control-side destination-policy
  resolver, `ResolveDestinationPolicy(DestinationPolicyRequest) (*DestinationPolicyResult, *ValidationError)`.
  Consumes the immutable `config.TenantSnapshot` captured at request start plus the
  validated target URL, requested fingerprint profile, and deployment upstream-proxy
  trust flags. Returns a `*strawpb.DestinationPolicy` bundle (matching the exact struct
  `internal/egress/executor.go` already consumes), the resolved ordered
  `[]*strawpb.InjectionOperation`, and the resolved fingerprint profile name.
  - Rejects URL userinfo defensively (already rejected earlier by `request.go`
    `validateURL`, re-checked here per the task's explicit instruction).
  - Normalizes the target host (lowercase, trailing-dot trim) and evaluates it against
    tenant deny rules: exact host-type deny/allow, and — for IP-literal targets — a
    fail-fast pre-dispatch check that reproduces `internal/egress/executor.go`'s
    default-deny CIDR/metadata-IP list plus `netip.Addr` private/loopback/link-local/
    multicast predicates, so Control's pre-check agrees with what Egress will actually
    enforce after DNS resolution. An explicit tenant allow-type cidr/ip deny rule is a
    true override at this layer (see "Known gap" below on Egress not yet honoring that
    the same way).
  - `cname`-type deny rules are not evaluated by Control (no DNS here); they are
    compiled into `DestinationPolicy.denied_cname_suffixes` for Egress.
  - Resolves ordered header-injection operations from all enabled tenant injection
    policies (sorted by policy ID for determinism), re-validating the Section 15 denied-
    header table and sensitive-header list, plus the checks write-time validation
    (`config_admin_handlers.go`) does not cover: duplicate `set` rejection across
    policies, CR/LF rejection in decoded values, and the aggregate size bound
    (`MaxInjectedHeaderBytes`, callers pass `ControlConfig.Transport.MaxFrameDataBytes`).
    Does not re-check the tenant_admin-only Authorization/Cookie restriction — that is
    an actor-authorization rule already enforced at write time; the resolver has no
    actor context for a stored snapshot.
  - Resolves the requested (or `"default"`) fingerprint profile against
    `snapshot.FingerprintProfiles`, requiring `Enabled && SupportedByWorker`, mapping
    any mismatch to `unsupported_fingerprint`.
  - Selects `DESTINATION_RESOLUTION_DIRECT_LOCAL` unless the caller passes
    `UpstreamProxyEnabled=true`, in which case an untrusted proxy is rejected with
    `destination_denied`. P0 has no upstream-proxy config surface anywhere in this repo,
    so real callers always pass `UpstreamProxyEnabled=false` today.
  - All failures return `*ValidationError` with the existing canonical codes
    (`invalid_request`, `destination_denied`, `header_injection_failed`,
    `unsupported_fingerprint`); messages/details never include the raw target URL/host.
- `internal/control/destination_policy_test.go` (new): focused resolver tests — host
  deny normalization, host allow override, metadata-IP default deny, private-range
  default deny, CIDR allow override, IPv4-mapped-IPv6 denial, CNAME rule compiled (not
  evaluated) at Control, fingerprint mismatch/disabled-profile rejection, injection
  ordering across policies, denied-header rejection, duplicate-`set` rejection, CRLF
  rejection, size-bound rejection, upstream-proxy trust policy (both outcomes),
  userinfo rejection, non-ASCII hostname rejection, and a public-safe-error-details
  check that denial errors never embed the raw host/path.
- `internal/control/config_resource_store.go`, `config_admin_handlers_test.go`,
  `handler_test.go`: lint-driven cleanup only — introduced `fingerprintProfileScopeGlobal`
  and reused the new `defaultFingerprintProfileName` / existing `urlSchemeHTTPS`
  constants to satisfy `goconst` after the new file added those constants (no behavior
  change).

## Decisions and flagged gaps

- **IDNA/punycode (task Stop Condition/explicit dependency question):** I asked the user
  whether to add `golang.org/x/net/idna` (not currently a real dependency of this
  module — it only appears transitively/unused) for proper IDNA normalization, per the
  AGENTS.md/skill rule to stop before adding a new dependency. No response arrived in
  time, so I proceeded with the documented safe default: hostnames containing any
  non-ASCII byte are rejected outright (`invalid_request`) rather than partially
  normalized. This is fail-closed and prevents Unicode-homoglyph SSRF bypass, but a
  tenant cannot reach a legitimate internationalized domain in P0. **No existing task
  file owns adding IDNA support** — if this is needed, please file a new task; I did not
  invent one to attach it to.
- **Known Egress gap (not fixed here — out of this task's file scope,
  `internal/control` only):** `internal/egress/executor.go`'s
  `validateCIDRPolicy`/`deniedByDefault` (task 11) treats `DestinationPolicy.AllowedCidrs`
  as a restrict-to-allowlist gate that is *still* separately subject to the
  private/loopback/link-local/metadata boolean checks and the static default-deny
  prefix list — it does not treat an allow-listed CIDR as an override of those checks.
  This resolver's `evaluateLiteralIPDeny`, by contrast, treats an explicit tenant
  allow-type deny rule as a true override (matching docs/planning/27's "denied by
  default unless a tenant admin explicitly allows them"). Net effect: a tenant that
  allow-lists a private/loopback/metadata IP will pass Control's pre-dispatch check
  today but can still be rejected by Egress after DNS resolution. Filed as
  `docs/tasks/p0/26-egress-destination-policy-precedence.md` (created after this
  handoff, once confirmed with the user that no existing task owned it) rather than
  silently deferring.
- **Known Egress gap (pre-existing, not introduced by this task):**
  `internal/egress/executor.go` does not yet read `DestinationPolicy.DeniedHostSuffixes`
  or `DeniedCnameSuffixes` at all — those bundle fields (correctly populated by this
  resolver) are not yet enforced downstream. This predates task 22 (it's a task 11 gap).
  Also folded into `docs/tasks/p0/26-egress-destination-policy-precedence.md`, since
  fixing it touches the same file and the same "does Egress honor what Control's bundle
  promises" question as the AllowedCidrs precedence gap above.
- **Default-port normalization:** the task steps ask for "default port" normalization
  in deny evaluation. The current deny-rule schema (task 20) stores plain
  hostname/IP/CIDR values with no port component, so default-port handling is a no-op
  given today's schema — documented as a comment in the resolver rather than
  implemented as dead code. Relevant only if a future task adds a port-scoped deny-rule
  type.

## Verification

```sh
go test ./internal/control/... -run TestResolveDestinationPolicy -v
make check
```

Result: all `TestResolveDestinationPolicy_*` tests pass (21 cases); `make check`
(`gofmt` + `go test ./...` + `golangci-lint run --max-issues-per-linter 0
--max-same-issues 0`) passes with 0 lint issues and all packages green.

## Reviewer Start Points

- `internal/control/destination_policy.go` — `ResolveDestinationPolicy` and its helpers.
- `internal/control/destination_policy_test.go` — resolver test coverage.
- `internal/egress/executor.go:516-638` — the Egress-side validator this resolver's
  output must agree with (see "Known Egress gap" above for the one place it doesn't yet).

## Remaining Work

- Wiring `ResolveDestinationPolicy` into the REST request handler and `RequestStart`
  construction is explicitly deferred to
  `docs/tasks/p0/24-control-request-dispatch-pipeline.md` (this task's own Out of Scope
  list).
- IDNA/punycode hostname support: no owning task exists; needs a new task if required
  (see "Decisions and flagged gaps" above).
- Reconciling `internal/egress/executor.go`'s `AllowedCidrs` precedence with this
  resolver's override semantics, and wiring `DeniedHostSuffixes`/`DeniedCnameSuffixes`
  enforcement: owned by `docs/tasks/p0/26-egress-destination-policy-precedence.md`.

## Blockers

- None for completing task 22 itself. The two flagged gaps above block full end-to-end
  enforcement of tenant "allow override" and cname/host-suffix deny rules once wired
  through Egress, but do not block this task's library-level acceptance criteria.

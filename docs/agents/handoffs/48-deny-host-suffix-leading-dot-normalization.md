# Handoff

Task: `docs/tasks/p0/48-deny-host-suffix-leading-dot-normalization.md`

## Changed

- `internal/control/config_admin_handlers.go` — `normalizeHostname` now trims leading dots too
  (`strings.Trim(..., ".")` instead of `strings.TrimSuffix(..., ".")`), so `.evil.example` stores and enforces as
  `evil.example`. `normalizeDenyRule` additionally rejects host/host_suffix/cname_suffix values that normalize to
  empty (`.`, `..`) with `errInvalidDenyRuleValue` → 400 — such a value would otherwise be stored as a rule no host
  can ever match, the exact failure mode this task closes.
- `internal/control/config_admin_handlers_test.go` — `TestDenyRuleLeadingDotNormalization`: leading-dot and dotless
  `host_suffix` normalize identically, the rule matches both the exact host and a subdomain, a leading-dot `host`
  rule matches its exact host, and dot-only values are rejected for all three hostname types.

**Decision: strip, not reject** (task's preferred option). A leading dot is a natural way to write a suffix;
silently-accepted-but-never-enforcing was the failure mode, and rejecting would break the natural spelling. The
degenerate dot-only case (normalizes to empty) is rejected with 400 since there is no matchable form to strip to.
`docs/planning/27` "Destination Deny Normalization" lists no leading-dot convention that contradicts this.

The fix is in the shared `normalizeHostname`, so `cname_suffix` values also get the leading-dot strip and
empty-rejection. This does not change `cname_suffix` *matching* semantics (out of scope) — it closes the same
inert-value hole for the third hostname-valued type through the same choke point.

No change was needed in `internal/control/destination_policy.go` or `internal/egress/executor.go`: the two
`hostMatchesSuffix` implementations are textually identical and both assume a dot-free stored suffix, which the
write-time normalization now guarantees (verifier confirmed parity at `destination_policy.go:354-364` /
`executor.go:722-732`).

## Acceptance Criteria Verdicts

From the independent verifier (fresh agent, task file + diff only):

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `.evil.example` rule denies `evil.example` and `x.evil.example`, test fails against old code | VERIFIED | `internal/control/config_admin_handlers.go:710-712` | `TestDenyRuleLeadingDotNormalization` (asserts `NormalizedHost == "evil.example"`, then `hostMatchesDenyRule` for both hosts; fails under old `TrimSuffix`-only code) |
| Leading-dot and dotless forms enforce identically | VERIFIED | same | same test (asserts `withDot.NormalizedHost == withoutDot.NormalizedHost`) |
| No host_suffix path accepts a never-matching value | VERIFIED | `config_admin_handlers.go:674-680` (single choke point: POST and PUT both route through `normalizeDenyRule`); 400 mapping at `:822-824` | dot-only rejection loop in same test |
| Stored `normalized_host` has no leading dot (strip) | VERIFIED | `config_admin_handlers.go:711` | same test |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/27` lowercase hostnames | already existed | `normalizeHostname` |
| `docs/planning/27` trailing-dot trimming | already existed (now trims repeated trailing dots too — strictly stronger, per task's "do not weaken" rule) | `config_admin_handlers.go:711` |
| Leading-dot handling (this task's gap) | implemented | `config_admin_handlers.go:711`, live check below |
| `docs/planning/27` IDNA/punycode | out of scope | `docs/tasks/p1/21-idna-hostname-support.md` |
| `docs/planning/27` ports/redirects/CNAME chains/IP literals/SNI/CONNECT normalization | out of scope (untouched; owned by tasks 22/26/43, done) | `internal/control/destination_policy.go`, `internal/egress/executor.go` |
| `docs/planning/26` deny-rule schema (`type`/`value`/`action`/`reason`) | already existed, unchanged | task 43 |

## Verification

```sh
go test ./internal/control ./internal/egress
make check
```

Result: all green, `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` → 0 issues. Verifier
independently re-ran the focused packages with `-count=1`: pass.

- Postgres-backed tests: not exercised — the diff touches no `postgres_*` files or `migrations/`; the change is
  in-process value normalization ahead of the (unchanged) store write.
- Live compose verification: **ran, surface-43 re-check passes.** Rebuilt `control` against this diff (fresh
  volumes, `STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=dev-admin-key-0001 docker compose up -d --build`, task 47's recipe).
  `POST /api/v1/config/deny-rules {"type":"host_suffix","value":".blocked.example","action":"deny"}` → 200 with
  stored value `blocked.example`; `GET https://sub.blocked.example/` → `destination_denied`;
  `GET https://blocked.example/` → `destination_denied`; `value:"."` → 400; `GET https://example.com/` → 200
  (no over-deny).

## Reviewer Start Points

- `internal/control/config_admin_handlers.go` (`normalizeHostname`, `normalizeDenyRule`)
- `internal/control/config_admin_handlers_test.go` (`TestDenyRuleLeadingDotNormalization`)

## Remaining Work

- None. Nothing is faked, stubbed, or deferred. Two verifier notes, accounted for here rather than deferred:
  - **No data backfill for pre-fix leading-dot rows**: none is needed — the only persistent environment is the
    local compose stack, whose volumes were recreated (`docker compose down -v`) during this task's live check, and
    every other environment runs migrations fresh. No deployment predates the fix, so no inert row can exist.
  - **Interior consecutive dots** (`evil..example`) are still accepted: pre-existing behavior, matchable in
    principle via the exact-match arm, explicitly outside this task's leading-dot scope, and not a
    silently-non-enforcing class (the request-side normalizer produces the same form).

## Blockers

- None. Work is committed.

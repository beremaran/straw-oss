# Lint Issues Triage & Priority List

This document provides a comprehensive triage and prioritization of the **215 lint issues** currently detected in the repository by `golangci-lint` (after applying exclusions for test packages and fixing Prometheus naming).

> [!NOTE]
> Some linters (like `errcheck`) report exactly 50 issues. This is due to the default `max-issues-per-linter` cap in `golangci-lint`. The actual number of violations for these linters may be higher.

---

## Executive Summary & Priority Table

| Priority | Linter | Issue Count | Description / Impact | Estimated Effort | Status / Recommended Strategy |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **High** | [staticcheck](https://staticcheck.dev/) | 14 | Logic bugs, deprecated APIs, unused variables. | Medium (2-4h) | Fix all occurrences. |
| **High** | `noctx` | 11 | Network/DB calls made without context propagation. | Medium (2-3h) | Fix all occurrences. |
| **High** | `errorlint` | 6 | Type assertion on wrapped errors (breaks error matching). | Low (1-2h) | Fix all occurrences. |
| **High** | `errcheck` | 50+ | Ignored error returns (leaks resources/silent failures). | Medium (2-4h) | Fix production; suppress minor test/defer errors. |
| **Medium** | `promlinter` | 0 (was 2) | Prometheus metric naming violations. | - | **Resolved** (metrics renamed). |
| **Medium** | `contextcheck` | 8 | Functions not passing context parameter. | Low (1-2h) | Fix all occurrences. |
| **Medium** | `errchkjson` | 8 | JSON marshaling/encoding errors not checked. | Low (1-2h) | Fix all occurrences. |
| **Medium** | `ineffassign` | 3 | Variable assigned but never used (potential logical bugs). | Very Low (15m) | Fix all occurrences. |
| **Medium** | `err113` | 31 (was 50+) | Inline dynamic errors (`errors.New`/`fmt.Errorf`). | Medium (2-3h) | Define sentinels in domain/pkg; configure linter. |
| **Low** | `cyclop` | 18 (was 23) | High cyclomatic complexity (poor maintainability). | High (1-2d) | Refactor complex functions (e.g. `Handle`). |
| **Low** | `nestif` | 5 | Deeply nested if statements. | Medium (2-4h) | Refactor with guard clauses / early exits. |
| **Low** | `testpackage` | 0 (was 50+) | Enforces black-box testing (`package_test`). | - | **Resolved / Excluded** (disabled for test files). |
| **Low** | `funlen` | 10 (was 50+) | Functions too long (LOC > 60). | Medium (2-4h) | Excluded for tests; refactor remaining production functions. |
| **Low** | `funcorder` | 33 | Ordering of functions/methods within files. | Low (1-2h) | Re-order functions stylistic choice. |
| **Low** | `nlreturn` | 6 | Missing blank line before return/break. | Low (15m) | Add newlines. |
| **Low** | `noinlineerr` | 3 | Inline error declarations in `if` statements. | Low (15m) | Reformat assignment. |
| **Low** | `nonamedreturns` | 1 | named return values. | Very Low (5m) | Remove named return. |
| **Low** | `usestdlibvars` | 2 | Magic HTTP status codes (use `http.StatusOK`). | Very Low (5m) | Replace `200` with standard constant. |
| **Low** | `whitespace` | 6 | Leading/trailing white spaces in functions/blocks. | Very Low (5m) | Remove whitespaces. |

---

## Detailed Triage & Recommendations

### Priority 1: High (Code Correctness, Resource Leaks, & Safety)

#### 1. `staticcheck` (14 issues)
* **Description:** Detects bugs, performance issues, and deprecated APIs.
* **Why it matters:** SA4009 (context overwrite) and SA4006 (unused variables) can hide logical bugs where execution is incorrect or context values are ignored. Deprecated APIs (like `rand.Seed`) can lead to compile failures in future Go versions.
* **Key Examples:**
  * [cmd/relay/main.go:156](file:///mnt/warehouse/projects/straw-proxy/cmd/relay/main.go#L156-L157) (`SA4009: argument ctx is overwritten before first use`)
  * [internal/endpoint/http/client.go:269](file:///mnt/warehouse/projects/straw-proxy/internal/endpoint/http/client.go#L269) (`SA1019: netErr.Temporary has been deprecated`)
  * [test/integration/session_test.go:24](file:///mnt/warehouse/projects/straw-proxy/test/integration/session_test.go#L24) (`SA1019: rand.Seed has been deprecated`)
* **Remediation Strategy:** Fix all occurrences. Replace deprecated calls (e.g. remove `rand.Seed` for Go 1.20+ or use `math/rand/v2`). Fix context assignments.

#### 2. `noctx` (11 issues)
* **Description:** Finds HTTP requests or network connections made without passing a context.
* **Why it matters:** In server-side code, dialing or listening without a context means calls cannot be canceled and do not honor timeouts, which can result in goroutine leaks and resource starvation under load.
* **Key Examples:**
  * [internal/infra/postgres/migrations.go:20](file:///mnt/warehouse/projects/straw-proxy/internal/infra/postgres/migrations.go#L20) (`db.Ping()` must use `PingContext`)
  * [pkg/validator/url.go:33](file:///mnt/warehouse/projects/straw-proxy/pkg/validator/url.go#L33) (`net.LookupIP(hostname)` should use `(*net.Resolver).LookupIPAddr` with context)
  * [internal/server/server_test.go:54](file:///mnt/warehouse/projects/straw-proxy/internal/server/server_test.go#L54) (`http.Get` without context)
* **Remediation Strategy:** Replace direct `net.Listen`, `net.Dial`, `net.LookupIP`, `db.Ping`, and `http.Get` with context-aware equivalents (`ListenConfig`, `DialContext`, `LookupIPAddr`, `PingContext`, `NewRequestWithContext`).

#### 3. `errorlint` (6 issues)
* **Description:** Identifies incorrect error checking patterns (e.g., comparing errors directly or using type assertions instead of `errors.Is`/`errors.As`).
* **Why it matters:** If an error is wrapped (common in modern Go packages using `%w`), direct type assertions (e.g., `err.(*MyError)`) or direct comparisons (e.g., `err == ErrMySentinel`) will fail, causing error handling branches to bypass incorrectly.
* **Key Examples:**
  * [internal/endpoint/fingerprint/registry_test.go:60](file:///mnt/warehouse/projects/straw-proxy/internal/endpoint/fingerprint/registry_test.go#L60) (`type assertion on error will fail on wrapped errors`)
  * [internal/endpoint/update/installer.go:89](file:///mnt/warehouse/projects/straw-proxy/internal/endpoint/update/installer.go#L89) (`non-wrapping format verb ... use %w`)
* **Remediation Strategy:** Update error comparisons to use `errors.As` or `errors.Is` and format wrapping errors with `%w` instead of `%v` in `fmt.Errorf`.

#### 4. `errcheck` (50+ issues)
* **Description:** Ensures error return values are not ignored.
* **Why it matters:** Ignoring errors from `Close` operations (e.g., `redisClient.Close`, `db.Close`, `resp.Body.Close`) or `Write` operations can lead to undetected failures, memory leaks, and connection leaks.
* **Key Examples:**
  * [cmd/relay/main.go:66](file:///mnt/warehouse/projects/straw-proxy/cmd/relay/main.go#L66) (`defer redisClient.Close()`)
  * [test/integration/helpers.go:389](file:///mnt/warehouse/projects/straw-proxy/test/integration/helpers.go#L389) (`defer resp.Body.Close()`)
* **Remediation Strategy:** 
  * For production files, check and log/handle the error (e.g., using a wrapper function for `defer Close`).
  * For test files where an unhandled `Close` error is not critical, either explicitly ignore them with `_ = client.Close()` or configure `errcheck` to ignore close functions.

---

### Priority 2: Medium (Metrics, Standard Patterns, & Inline bugs)

#### 1. `promlinter` (0 issues - Resolved)
* **Description:** Enforces Prometheus best practices for metric naming conventions.
* **Status:** Resolved. The metrics `endpoint_tls_fingerprint_used` and `endpoint_fingerprint_deprecated_used` were renamed to `endpoint_tls_fingerprint_used_total` and `endpoint_fingerprint_deprecated_used_total` respectively.

#### 2. `contextcheck` (8 issues)
* **Description:** Warns if a function should pass context but doesn't.
* **Why it matters:** Ensures context propagation is not broken midway down a call tree.
* **Key Examples:**
  * [cmd/relay/main.go:160](file:///mnt/warehouse/projects/straw-proxy/cmd/relay/main.go#L160) (`tryStopServer` uses a new context instead of inherited `ctx`)
* **Remediation Strategy:** Pass the existing context to the methods instead of calling `context.Background()` or omitting it.

#### 3. `errchkjson` (8 issues)
* **Description:** Checks for unchecked errors during JSON marshaling/encoding.
* **Why it matters:** Marshaling can fail under edge cases (e.g. recursive definitions, unsupported channel/function types).
* **Key Examples:**
  * [internal/server/admin/handlers/fingerprints_test.go:113](file:///mnt/warehouse/projects/straw-proxy/internal/server/admin/handlers/fingerprints_test.go#L113) (`body, _ := json.Marshal(preset)`)
* **Remediation Strategy:** Handle the error return or replace it with `require.NoError(t, err)` in tests.

#### 4. `ineffassign` (3 issues)
* **Description:** Identifies variables that are assigned but never read.
* **Why it matters:** Usually indicates a typo or redundant logic where the developer expected to use a variable but didn't.
* **Key Examples:**
  * [internal/service/ratelimit/limiter_test.go:132](file:///mnt/warehouse/projects/straw-proxy/internal/service/ratelimit/limiter_test.go#L132) (`allowed, res, err := ...` where `res` is unused)
* **Remediation Strategy:** Delete the unused variable assignment or check/use it.

#### 5. `err113` (31 issues)
* **Description:** Disallows dynamic error generation (`errors.New` or `fmt.Errorf`) on the fly.
* **Why it matters:** Makes it difficult for callers to inspect the cause of an error. Errors should ideally be package-level sentinels.
* **Key Examples:**
  * [internal/domain/tag.go:16](file:///mnt/warehouse/projects/straw-proxy/internal/domain/tag.go#L16) (`fmt.Errorf("empty tag string")`)
* **Remediation Strategy:** Define sentinels (e.g. `var ErrEmptyTag = errors.New("empty tag string")`) at the top of package files, then return/wrap those sentinels.

---

### Priority 3: Low (Complexity, Code Layout, & Test Styling)

#### 1. `cyclop` (18 issues)
* **Description:** Checks cyclomatic complexity against a threshold (configured at 10).
* **Why it matters:** Code with high complexity is harder to trace, debug, and test.
* **Key Examples:**
  * [internal/server/handlers/relay.go:70](file:///mnt/warehouse/projects/straw-proxy/internal/server/handlers/relay.go#L70) (`calculated complexity for function Handle is 54`)
* **Remediation Strategy:** Refactor large functions into smaller, modular helpers.

#### 2. `nestif` (5 issues)
* **Description:** Checks for deeply nested if statements (threshold is usually 5).
* **Why it matters:** Simplifies nested control flow.
* **Key Examples:**
  * [internal/server/handlers/relay.go:243](file:///mnt/warehouse/projects/straw-proxy/internal/server/handlers/relay.go#L243) (`nestif` complexity 11)
* **Remediation Strategy:** Simplify nested blocks using early returns, guard clauses, or lookup tables.

#### 3. `testpackage` (0 issues - Excluded)
* **Status:** Excluded for test files. Unit tests can now remain in the same package as their production counterparts, allowing access to internal/private helpers without extra boilerplate.

#### 4. `funlen` (10 issues)
* **Description:** Enforces maximum function length (usually 60 lines of code or 40 statements).
* **Why it matters:** Large functions are hard to follow.
* **Key Examples:**
  * [internal/endpoint/fingerprint/presets.go:28](file:///mnt/warehouse/projects/straw-proxy/internal/endpoint/fingerprint/presets.go#L28) (`registerChromePresets` has 111 lines)
* **Remediation Strategy:** Since it is excluded for test files, we only need to address the remaining 10 long functions in production code (most are registry setup functions which can be kept as-is or divided into smaller setup blocks).

#### 5. Others (Stylistic / Formatting)
* **Linters:** `funcorder`, `nlreturn`, `noinlineerr`, `nonamedreturns`, `usestdlibvars`, `whitespace`.
* **Remediation Strategy:** Apply trivial formatting changes. For `usestdlibvars`, replace `"200"` with `http.StatusOK`.

---

## Action Plan & Recommendations

1. **Done - Update `.golangci.yml` Exclusions:** 
   Excluded `testpackage`, `funlen`, `err113`, and `cyclop` for all `_test.go` files using `golangci-lint v2` syntax. This dropped active issue count from 331 to 215.
2. **Done - Fix Prometheus Metric Naming:**
   Renamed `endpoint_tls_fingerprint_used` and `endpoint_fingerprint_deprecated_used` to include the `_total` suffix.
3. **Phase 1 (High Priority - Correctness):** Fix `staticcheck`, `errorlint`, `noctx`, and production-side `errcheck` violations first.
4. **Phase 2 (Medium Priority - Clean patterns):** Address missing context propagations (`contextcheck`) and declare sentinel errors (`err113`).
5. **Phase 3 (Low Priority - Refactoring):** Address complexity issues (`cyclop`, `nestif`) in core handlers.

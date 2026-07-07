# Handoff

Task: `docs/tasks/p0/08-worker-registration-heartbeat-state.md`

## Changed

- `api/proto/straw/v1/registration_sign.go` (new): canonical registration
  signing payload + `SignRegistration` / `VerifyRegistrationSignature`
  (ed25519, stdlib). Domain-separated payload binds `worker_id`,
  `credential_id`, `executor_type`, and protocol major/minor. Lives in
  `strawpb` so both the egress signer and the control verifier share one
  definition without a layering cycle.
- `internal/natsx/natsx.go`: added `ControlInboxPrefix()` (`_INBOX.ctl`),
  `WorkerInboxPrefix(worker_id)` (`_INBOX.wrk.<worker_id>`), and exported
  `ValidateSubjectToken` for the scoped request/reply reply-inbox prefixes in
  the NATS ACL table.
- `internal/control/worker_credential_store.go`: added
  `WorkerCapabilities` type and an `AllowedCapabilities` field on
  `WorkerCredential` so registration can enforce capability scope. Empty
  lists mean "unrestricted" (P0 permissive default; the tenant_admin create
  API in task 07 does not author these yet, so existing task-07 behavior and
  tests are unchanged).
- `internal/control/worker_registry.go` (new): `WorkerRegistry` — the
  in-process worker state service. Registration validation, heartbeat
  processing (Control receive-time liveness), runtime-state derivation,
  duplicate/stale session handling, cooldown, admin overrides, tenant
  eligibility, and the platform/tenant listing read models.
- `internal/control/worker_handlers.go` (new): the runtime worker admin HTTP
  surface on `AdminHandlers` — `GET /workers`, global disable/enable/
  drain/undrain, and tenant disable/enable/drain/undrain.
- `internal/control/admin_handlers.go`: added `Workers *WorkerRegistry` to
  `AdminHandlers`.
- `internal/egress/registration.go` (new): egress-side `BuildRegisterRequest`
  (signs the request), `BuildHeartbeat`, and `Identity.InboxPrefix()`.
- `cmd/control/main.go`: constructs a shared `WorkerCredentialStore` +
  `WorkerRegistry` and registers the `/api/v1/admin/workers*` routes.
- Tests: `worker_registry_test.go`, `worker_handlers_test.go`,
  `internal/natsx/inbox_test.go`, `internal/egress/registration_test.go`.

## Verification

```sh
make check
```

Result: pass (gofmt clean; `go test ./...` all green).

Focused runs: `go test ./internal/control/ ./internal/natsx/ ./internal/egress/ ./api/...` pass.

Note on `golangci-lint`: not part of the repo's gate (`make check` = gofmt +
tests) and there is no CI workflow. The existing codebase already has a large
pre-existing lint baseline (273 issues across prior-task files); new code
follows the same conventions as the surrounding handlers. Not addressed here.

## State names and timeout constants (as requested)

Runtime states (`WorkerRuntimeState`): `unregistered`, `registered`, `ready`,
`degraded`, `unhealthy`, `unavailable`, `dead`, `draining`, `cooldown`,
`duplicate_replaced`. Admin states (`AdminState`): `enabled`, `disabled`.

Timing defaults (`DefaultWorkerTimings`, from docs/planning/11):

- availability timeout 15s (→ `unavailable`), dead timeout 30s (→ `dead`),
- duplicate-session grace 10s, cooldown 3 failures / 60s window, cooldown
  duration 30s.

Runtime-state precedence when deriving state: `dead` > `unavailable` >
(`registered` if never heartbeated) > `cooldown` > `draining` > health
(`ready`/`degraded`/`unhealthy`). Eligibility precedence
(`EligibleForTenant`): global disable > tenant disable > drain (global or
tenant) > runtime routability (`ready`/`degraded`).

## How test time is controlled

`WorkerRegistry` takes an injectable `now func() time.Time`. Tests use
`fakeClock` (in `worker_registry_test.go`) and call `Advance` to cross the
15s/30s/cooldown thresholds deterministically. `cmd/control` passes `nil`,
which defaults to `time.Now`.

## Reviewer Start Points

- `internal/control/worker_registry.go` (core logic)
- `internal/control/worker_handlers.go` (RBAC + tenant redaction)
- `api/proto/straw/v1/registration_sign.go` (signing contract)

## Remaining Work / Deferred (documented, not blocking)

- Live NATS transport binding for register/heartbeat request/reply is not
  wired: the repo has no NATS client dependency yet (natsx is helpers/
  validation only, consistent with tasks 03/06/07). This task implements the
  registration/heartbeat *processing* + subject/inbox helpers the future
  transport will call. Assignment/stream transport is task 10.
  [Update 2026-07-07 sweep: resolved — `docs/tasks/p0/16-nats-client-foundation.md` added the live NATS client,
  `docs/tasks/p0/17-worker-registration-heartbeat-over-nats.md` wired register/heartbeat request/reply, and
  `docs/tasks/p0/23-egress-assignment-execution-loop.md` wired assignment/stream transport.]
- Runtime state is in-process only (no Redis/TTL yet); Redis-backed worker session/heartbeat/load and cooldown state
  is now owned by `docs/tasks/p0/45-redis-backed-worker-runtime-state.md`.
- Registration signature has no per-registration nonce (Core NATS
  request/reply gives no nonce channel in P0); it proves credential
  possession, with NATS subject ACLs + credential status as the outer
  controls. Documented in `registration_sign.go`.
  [Update 2026-07-07 sweep: resolved — `docs/tasks/p0/35-worker-registration-replay-and-identity-key.md` added
  `nonce` and `issued_at_unix_ms` to registration signing, Redis-backed nonce consumption, and persistent egress
  identity keys.]
- Capability-scope authoring via `POST /worker-credentials` (task 07 surface)
  is not added; `AllowedCapabilities` defaults to unrestricted.
  [Update 2026-07-06 sweep: resolved — `POST /worker-credentials` accepts and validates
  `allowed_capabilities` (`internal/control/admin_handlers.go:1363,1407`), landed with
  `docs/tasks/p1/08-multi-tenant-worker-credentials.md`.]

## Blockers

- None.

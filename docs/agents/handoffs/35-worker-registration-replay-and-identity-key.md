# Handoff

Task: `docs/tasks/p0/35-worker-registration-replay-and-identity-key.md`

## Changed

- `api/proto/straw/v1/straw.proto`, `straw.pb.go`: added `nonce` (`bytes`, field 17) and `issued_at_unix_ms`
  (`int64`, field 18) to `RegisterRequest`. `int64` matches the existing `deadline_unix_ms`/`worker_timestamp_ms`
  unix-millis convention rather than the task text's literal `uint64`.
- `api/proto/straw/v1/registration_sign.go`: `RegistrationSigningPayload` now binds a length-prefixed nonce and the
  issued-at timestamp after the existing worker_id/credential_id/executor_type/protocol fields. Added
  `appendInt64`/`appendUint64` helpers.
- `docs/planning/13-protobuf-contract.md`, `docs/planning/a-reconciliation-notes.md`,
  `docs/planning/24-static-configuration.md`: recorded the field additions and the new
  `control.worker.registration_*` config keys; noted (pre-existing, not caused by this task) that
  `egress.worker_id`/`egress.credential_id`/the new `egress.private_key_ed25519_env` are flat keys, not nested under
  `egress.credential` as the doc's canonical table shows — unowned reconciliation.
- `internal/egress/registration.go`: `BuildRegisterRequest` now returns `(*RegisterRequest, error)`, populating a
  16-byte `crypto/rand` nonce and `time.Now().UnixMilli()` issued-at. Updated the one caller
  (`internal/egress/runtime.go`) and its test.
- `internal/config/config.go`: added `EgressConfig.PrivateKeyEd25519Env` (required, validated) and
  `ControlConfig.Worker` (`ControlWorkerConfig`: `RegistrationClockSkewMS` default 60000,
  `RegistrationNonceTTLMS` default 300000, `RegistrationFailOpenOnRedisOutage` default `false`).
- `cmd/egress/main.go`: loads the persistent key from the configured env var (32-byte seed or 64-byte full key,
  base64-standard) instead of generating a throwaway keypair every boot.
- `internal/control/worker_nonce.go` (new): `WorkerNonceStore` interface and `RedisWorkerNonceStore` (`SET NX` with
  TTL, key `straw:workernonce:<credential_id>:<base64url(nonce)>`).
- `internal/control/worker_registry.go`: added `WorkerRegistrationPolicy`, `DefaultWorkerRegistrationPolicy` (skew
  60s, TTL 5m), `WorkerRegistry.SetNonceStore`, and `checkRegistrationReplay` (issued-at skew check always active;
  nonce consume/replay/outage-policy check only when a nonce store is wired). New reject reasons:
  `RejectStaleIssuedAt`, `RejectNonceReplayed`, `RejectNonceStoreUnavailable`. Skew is enforced even without
  `SetNonceStore` (test harnesses populate valid issued-at values; only nonce replay/outage checks are skipped when
  unwired).
- `cmd/control/main.go`: wires `RedisWorkerNonceStore` into `WorkerRegistry` from `ControlConfig.Worker`
  (`wireWorkerRegistrationReplayProtection`); calls a new dev-credential bootstrap on startup
  (`bootstrapWorkerCredential`).
- `internal/control/bootstrap.go`: added `BootstrapWorkerCredentialFromEnv` (idempotent, mirrors the existing
  `BootstrapFromEnv` API-key bootstrap) and `DevWorkerIDEnvVar`/`DevWorkerPublicEd25519EnvVar` env var names. Renamed
  away from an initial `BootstrapWorkerCredential...`/`BootstrapWorkerPublicKey...` naming because gosec G101 flags
  the `p`+`W` boundary formed by `Bootstrap`+`Worker` concatenated (a real false positive, confirmed by bisection —
  not about "Credential"/"Key" substrings as first assumed).
- `internal/control/postgres_worker_credential_store.go`: `Get` now maps `pgx.ErrNoRows` to
  `ErrWorkerCredentialNotFound` (it previously wrapped every error including "no rows," which broke the new
  bootstrap's existence check — the store's `Revoke` has the same unmapped gap but is unrelated to this task).
- `deploy/docker/egress.json`, `docker-compose.yml`, `deploy/docker/README.md`: seeded a **dev-only** ed25519
  keypair end-to-end — egress's `private_key_ed25519_env` config points at
  `STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64` (set in compose); Control's `STRAW_BOOTSTRAP_WORKER_CREDENTIAL_ID`/
  `STRAW_BOOTSTRAP_WORKER_PUBLIC_KEY_ED25519_BASE64` seed the matching credential. Dropped the README's "will not
  succeed out of the box" caveat.
- Tests: `internal/control/worker_nonce_test.go` (new — Redis-backed, skips if unreachable, same pattern as
  `sticky_redis_test.go`), `internal/control/worker_registry_test.go` (replay/skew/outage/fail-open cases + a
  `fakeNonceStore` so those don't require live Redis), `internal/control/worker_handlers_test.go`,
  `internal/control/worker_nats_test.go` (switched fixed epoch fake clocks to `time.Now()` so the
  `egress.Register`-signed issued-at, which uses real wall-clock time, doesn't fail the skew check against an
  unrelated fake clock), `internal/egress/registration_test.go`, `internal/config/config_test.go`.

## Verification

```sh
make check
```

Result: pass (`gofmt`, `go test ./...`, `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` — 0
issues).

Additionally ran a live `docker compose up -d --build` end-to-end: confirmed via container logs (no more `no
responders`/rejection lines after the worker's first clean registration), `worker_credentials` row seeded with the
compose-baked keypair, and `redis-cli KEYS 'straw:workernonce:*'` showing consumed nonces for the seeded
credential_id. Compose was torn down afterward (`docker compose down`, no `-v`).

## Reviewer Start Points

- `internal/control/worker_registry.go` (`checkRegistrationReplay`, `SetNonceStore`)
- `internal/control/worker_nonce.go`
- `api/proto/straw/v1/registration_sign.go`
- `cmd/control/main.go` (`wireWorkerRegistrationReplayProtection`, `bootstrapWorkerCredential`)
- `cmd/egress/main.go` (`loadWorkerPrivateKey`)
- `deploy/docker/README.md` ("Worker provisioning")

## Remaining Work

- None for this task's scope. Two adjacent, unowned gaps noted above for future reconciliation (not blocking, not
  faked/stubbed by this task): the `egress.credential.*` nested-vs-flat config doc mismatch, and
  `postgresWorkerCredentialStore.Revoke` not mapping `pgx.ErrNoRows` (pre-existing, unrelated to this task's
  acceptance criteria).
- 2026-07-05: P0-audit gap closed — `postgresWorkerCredentialStore.Revoke` now maps `pgx.ErrNoRows` to
  `ErrWorkerCredentialNotFound` (`internal/control/postgres_worker_credential_store.go:135-137`), so the handler
  returns 404 instead of 500 for missing/already-revoked credentials. Same latent bug fixed in the sibling
  `postgresAPIKeyStore.Revoke` (`internal/control/postgres_apikey_store.go:141-143`, maps to `ErrAPIKeyNotFound`).
  Proven by `TestPostgresWorkerCredentialStoreRevokeMissingNotFound` in `internal/control/postgres_store_test.go`.

## Blockers

- None.

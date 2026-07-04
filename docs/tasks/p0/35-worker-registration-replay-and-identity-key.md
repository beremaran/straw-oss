# 35 - Worker Registration Replay Protection and Persistent Identity Key

Status: not started

## Objective

Complete the `docs/planning/27` "Worker Credential Signing" control: bind a nonce and issued-at timestamp into the
signed registration token with Redis-backed replay protection (fail-closed by default), and give the egress worker a
configured persistent Ed25519 private key so a live worker can register against a pre-seeded credential.

## Context (gap being closed)

The 2026-07-04 review follow-up found the Ed25519 signature path itself is implemented and enforced:
`RegistrationSigningPayload`/`SignRegistration`/`VerifyRegistrationSignature`
(`api/proto/straw/v1/registration_sign.go`) sign a domain-separated payload binding
`worker_id`/`credential_id`/`executor_type`/protocol version, and `WorkerRegistry.Register` verifies it against the
credential's stored public key alongside status/scope/capability checks. Two `docs/planning/27` requirements are
missing:

1. The signed payload carries no nonce or issued-at, so a captured `RegisterRequest` authorizes replays forever. The
   comment in `registration_sign.go` documents this gap. `docs/planning/27` requires nonces stored in Redis with TTL
   scoped by `credential_id`, registration failing closed when Redis is unavailable (fail-open only by explicit
   deployment opt-in), and configurable clock-skew tolerance defaulting to 60 seconds. The nonce travels inside the
   signed token, so Core NATS request/reply needs no extra channel.
2. `cmd/egress/main.go` generates a throwaway keypair each boot and no config field can load a persistent key, so a
   live worker's signature can never match a seeded credential's `public_key_ed25519_base64`.
   `deploy/docker/README.md` documents that compose registration "will not succeed out of the box" for exactly this
   reason.

`docs/planning/11` lists the signed token and the validation set Control already performs; `docs/planning/27` adds the
nonce/replay requirements on top (union, not conflict). Existing scope/capability/protocol validation must not change.

## Required Planning Docs

- `docs/planning/27-security-controls.md` (Worker Credential Signing, nonce replay protection)
- `docs/planning/11-worker-discovery-and-health.md` (registration flow and validation set)
- `docs/planning/13-protobuf-contract.md` (`RegisterRequest`, `signed_token`)
- `docs/planning/12-nats-protocol.md` (registration subjects, envelope validation)
- `docs/planning/21-state-and-storage.md` (Redis runtime state)
- `docs/planning/24-static-configuration.md` (egress config surface)

## Prerequisites

- Task 16 and Task 17 completed (NATS registration path).
- Task 18 completed (worker credential store).
- Task 21 completed (live Redis wiring).

## Out of Scope

- Do not implement key rotation or a key-delivery API; the key is static config in P0.
- Do not implement multi-tenant worker credentials (P1 task 08).
- Do not change the existing scope/capability/protocol registration validation.

## Expected Files

- Modify: `api/proto/straw/v1/straw.proto` (add `nonce` and `issued_at_unix_ms` to `RegisterRequest`) and regenerate
  via buf.
- Modify: `docs/planning/13-protobuf-contract.md` and `docs/planning/a-reconciliation-notes.md` (record the
  planning/27-driven field additions so the contract doc stays canonical).
- Modify: `api/proto/straw/v1/registration_sign.go` (include nonce and issued-at in the signed payload; update the
  comment that documents the replay gap).
- Modify: `internal/egress/registration.go` (populate a `crypto/rand` nonce and issued-at when building the request).
- Modify: `internal/config/config.go` (egress `private_key_ed25519_base64` field or key-file path, with validation).
- Modify: `cmd/egress/main.go` (load the configured key; stop generating a throwaway keypair).
- Create: `internal/control/worker_nonce.go` (Redis nonce store: `SET NX` with TTL, scoped by `credential_id`;
  fail-closed default, explicit fail-open flag).
- Modify: `internal/control/worker_registry.go` (verify issued-at within skew and consume the nonce after signature
  verification; new reject reasons).
- Modify: `cmd/control/main.go` (wire the nonce store, skew, and TTL into the registry).
- Modify: `deploy/docker/egress.json`, `deploy/docker/README.md`, and a dev seed for the matching credential so
  compose registration succeeds out of the box.
- Test: `internal/egress/registration_test.go`, `internal/control/worker_registry_test.go`,
  `internal/control/worker_nats_test.go`, nonce-store test.

## Steps

- [ ] Read all required planning docs.
- [ ] Add `nonce` and `issued_at_unix_ms` to `RegisterRequest`, regenerate, and record the addition in
      `docs/planning/13` and `docs/planning/a-reconciliation-notes.md`.
- [ ] Include nonce and issued-at in `RegistrationSigningPayload`; keep the domain-separation prefix and the
      unambiguous field joining.
- [ ] Populate nonce (crypto/rand) and issued-at on the egress side.
- [ ] Add the egress private-key config field with validation; load it in `cmd/egress/main.go` instead of generating a
      keypair.
- [ ] Add the Redis nonce store: reject a seen nonce, TTL per `docs/planning/27`, scoped by `credential_id`;
      registration fails closed on Redis error unless an explicit fail-open config flag is set (default off).
- [ ] Enforce issued-at within a configurable skew (default 60s) and consume the nonce in `WorkerRegistry.Register`
      after signature verification, with distinct reject reasons.
- [ ] Seed a dev key and matching worker credential for docker compose; update `deploy/docker/README.md` to drop the
      "will not succeed out of the box" caveat.
- [ ] Add tests for: replayed nonce rejected despite a valid signature; stale issued-at rejected; issued-at within
      skew accepted; Redis outage fails closed (and fail-open only when configured); configured-key registration
      succeeds against the seeded credential; existing scope/capability rejections unchanged.
- [ ] Run the focused tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./api/... ./internal/egress ./internal/control`
- `make check`

## Acceptance Criteria

- A replayed `RegisterRequest` (same nonce) is rejected even though its signature is valid; nonces expire only by TTL.
- An issued-at outside the configured skew (default 60s) is rejected.
- With Redis unavailable, registration fails closed by default; fail-open requires an explicit config flag.
- The egress worker signs with the configured persistent key, and the compose stack's worker registers successfully
  against the seeded credential.
- Existing credential-status/scope/capability/protocol validation behavior is unchanged.

## Handoff Notes

- Document the final signed-payload field order, the nonce TTL/skew defaults, and the fail-policy flag.
- Document the dev key/credential seeding used by compose and why it is dev-only.

## Stop Conditions

- Stop if amending `docs/planning/13`'s `RegisterRequest` is disputed or conflicts with another planning doc;
  reconcile before coding.
- Stop before adding key rotation or multi-tenant worker credentials.
- Stop if a deferral would have no owning task file.

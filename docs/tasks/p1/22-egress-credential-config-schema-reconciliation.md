# 22 - Egress Credential Config Schema Reconciliation (flat vs nested)

Status: not started

## Objective

The egress credential configuration has exactly one canonical shape across code, deploy fixtures, and the planning
doc. After this task, `docs/planning/24-static-configuration.md` no longer describes two conflicting shapes for the
worker identity/credential keys, the config loader accepts only the canonical shape, and a config test proves the
canonical form round-trips and the non-canonical `egress.credential.*` nested form does not silently "work."

## Context (gap being closed)

The P0 audit and the task-35 handoff (`docs/agents/handoffs/35-worker-registration-replay-and-identity-key.md`,
"Remaining Work") flagged a config-schema/doc mismatch and explicitly recorded it as having **no owning task**. This
task is the owner.

Current-code evidence:

- `internal/config/config.go:138-148` — `EgressConfig` declares `WorkerID` (`worker_id`), `CredentialID`
  (`credential_id`), and `PrivateKeyEd25519Env` (`private_key_ed25519_env`) as **flat top-level** JSON keys under
  `egress`.
- `deploy/docker/egress.json:4-6` — the working deploy fixture uses the **flat** keys (`worker_id`,
  `credential_id`, `private_key_ed25519_env`), matching the code and loading successfully in compose.
- `docs/planning/24-static-configuration.md` is internally inconsistent:
  - the key table lists both the **nested** `egress.credential.credential_id_env` /
    `egress.credential.private_key_env` (lines ~77-78) **and** the flat `egress.private_key_ed25519_env` (line ~79);
  - the Egress Config Example shows the flat keys (lines ~219-221) **and** a separate nested `credential:` block
    (lines ~231-233);
  - the note at lines ~109-112 already concedes the flat shape is what is implemented and states the reconciliation
    "has no owning task."

Decision (reconciliation direction — do not re-litigate): **the flat shape is canonical; correct the planning doc to
the flat shape.** Rationale: task 35 deliberately chose flat keys "for consistency with the fields it sits next to,"
the running deploy fixture and the validated loader are already flat, and the planning doc itself documents the flat
shape as the implemented one. The generic "planning doc wins" rule does not force the nested shape here because the
planning doc is self-contradictory and already records the flat shape as intended — the reason-otherwise condition in
the AGENTS.md rule is met. If, while executing, you find a concrete correctness reason the nested shape must win
instead, that is a stop-and-ask condition, not a silent reversal.

## Required Planning Docs

- `docs/planning/24-static-configuration.md` — the egress key table (lines ~70-107), the flat-vs-nested note
  (lines ~109-112), and the Egress Config Example (lines ~214-253). This is the authoritative egress config schema
  doc; no other planning doc defines these keys.

## Prerequisites

- P0 task 35 completed (added `egress.private_key_ed25519_env`; its handoff is the source of this gap). Done.

## Out of Scope

- Do not change any credential *values*, secret-handling behavior, or the env-var indirection convention (secrets stay
  in env vars, never inline) — only the config *key shape* and its documentation.
- Do not touch the `egress.capabilities.*`, `egress.outbound_*`, `egress.dns.*`, or NATS config keys; this task is
  scoped to the three identity/credential keys (`worker_id`, `credential_id`, `private_key_ed25519_env`).
- Do not add a backward-compat shim that accepts both shapes; a single canonical shape is the whole point.

## Expected Files

- Modify: `docs/planning/24-static-configuration.md` (remove the nested `egress.credential.*` table rows and the
  nested `credential:` example block; delete the "has no owning task" reconciliation note or replace it with a
  one-line statement that the flat shape is canonical and cite this task).
- Verify/keep: `internal/config/config.go` (already flat — confirm no change needed; only touch if you find the code
  drifted from the flat canonical shape).
- Verify/keep: `deploy/docker/egress.json` (already flat — confirm it matches the canonical shape).
- Test: `internal/config/config_test.go` (add the round-trip + rejection cases described below).

## Steps

- [ ] Read the required planning doc sections listed above.
- [ ] Confirm the flat shape is fully consistent across `internal/config/config.go`, `deploy/docker/egress.json`, and
      the compose stack; record any drift.
- [ ] Edit `docs/planning/24-static-configuration.md`: delete the nested `egress.credential.credential_id_env` /
      `egress.credential.private_key_env` table rows and the nested `credential:` block in the Egress Config Example;
      remove or rewrite the "no owning task" note (lines ~109-112) to state the flat shape is canonical.
- [ ] In `internal/config/config_test.go`, add a test that (a) a config using the flat `worker_id` / `credential_id`
      / `private_key_ed25519_env` keys loads and validates and the loaded struct fields round-trip, and (b) a config
      that puts the credential fields under a nested `egress.credential.*` object (and omits the flat keys) fails
      `LoadEgress` validation with the existing "missing required field" error rather than silently loading empty
      credentials.
- [ ] Run focused config tests (`go test ./internal/config`), then `make check`.
- [ ] Write a handoff note recording the reconciliation direction and confirming the flag/note removal.

## Tests

- `go test ./internal/config`
- `make check`

## Acceptance Criteria

- `grep -n "egress.credential" docs/planning/24-static-configuration.md` returns no key-table row and no example
  block presenting the nested credential shape as canonical (proven by the grep).
- The "Reconciling the nested `egress.credential.*` shape ... has no owning task" sentence no longer exists in
  `docs/planning/24-static-configuration.md`; any surviving mention names this task (`p1/22`) as owner.
- A `config_test.go` case loads the flat-key egress config and asserts `WorkerID`, `CredentialID`, and
  `PrivateKeyEd25519Env` are populated (round-trip proven by test).
- A `config_test.go` case feeds the nested `egress.credential.*` form and asserts `LoadEgress` returns the existing
  missing-required-field validation error (non-canonical shape does not silently succeed, proven by test).
- `deploy/docker/egress.json` uses only the flat keys and still loads (no nested `credential` object).
- `make check` passes.

## Handoff Notes

- Record that the flat shape was chosen as canonical and why (task-35 precedent + running fixture + validated loader),
  so a future reader does not re-open the direction question.
- Confirm the task-35 handoff's "no owning task" flag for this gap is now closed by this task.

## Stop Conditions

- Stop and ask if you find a concrete correctness or security reason the nested `egress.credential.*` shape must be
  canonical instead of flat (this reverses the decided direction).
- Stop if reconciling would require touching credential secret-handling behavior rather than key shape.
- Stop if a deferral would have no owning task file.

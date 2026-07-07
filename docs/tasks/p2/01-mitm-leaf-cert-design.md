# 01 - MITM Leaf Certificate Design

Status: done

## Objective

Specify MITM leaf-certificate storage, encryption, cache miss coalescing, and unique-SNI flood controls before MITM
implementation starts.

## Required Planning Docs

- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/32-open-decisions.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Decision `P2 MITM Private-Key Storage Policy` resolved.

## Out of Scope

- Do not implement MITM ingress.
- Do not generate production CA material.
- Do not create certificate cache code.

## Expected Files

- Create: `docs/planning/c-mitm-leaf-certificate-design.md`

## Steps

- [x] Read all required planning docs.
- [x] Record the resolved private-key storage policy and rejected options.
- [x] Specify public certificate caching and private-key storage/encryption behavior.
- [x] Specify TTL rules bounded by certificate validity days.
- [x] Specify local singleflight, Redis distributed lock, bounded generation concurrency, and per-tenant/global
      unique-SNI flood limits.
- [x] Specify rotation, cache miss, and encrypted-at-rest tests.
- [x] Add MITM certificate test rows needed before implementation.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Documentation/spec review.
- `make check`

## Acceptance Criteria

- Certificate private-key handling is fully decided before code work starts.
- CPU/flood controls are explicit.
- No production code is changed.

## Handoff Notes

- Link the design and the resolved open decision.

## Stop Conditions

- Stop if `P2 MITM Private-Key Storage Policy` is unresolved.
- Stop if a deferral would have no owning task file.

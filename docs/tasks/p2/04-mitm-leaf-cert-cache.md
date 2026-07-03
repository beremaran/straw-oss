# 04 - MITM Leaf Certificate Cache

Status: not started

## Objective

Implement leaf certificate generation, cache/storage, coalescing, and flood controls exactly as specified by task 01.

## Required Planning Docs

- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Task 01 completed.
- Task 02 completed.
- Task 03 completed.

## Out of Scope

- Do not change the private-key storage policy chosen in task 01.
- Do not implement MITM request dispatch beyond certificate lookup/generation.

## Expected Files

- Create or modify: MITM certificate cache package.
- Create or modify: Redis key/lock helpers if needed.
- Test: certificate cache tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Generate per-SNI leaf certificates signed by the configured CA.
- [ ] Store public certificate bytes and private-key material according to task 01.
- [ ] Enforce TTL no longer than configured cert validity days.
- [ ] Implement local singleflight and Redis distributed locking.
- [ ] Enforce bounded generation concurrency plus per-tenant and global unique-SNI limits.
- [ ] Add tests for cache hits, cache misses, encrypted storage, rotation, flood limits, Redis lock loss, and TTL.
- [ ] Run focused certificate cache tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused MITM certificate cache tests.
- `make check`

## Acceptance Criteria

- Leaf generation is coalesced and bounded.
- Stored private keys, if any, follow the resolved encryption policy.
- Unique-SNI floods cannot saturate Control CPU unchecked.

## Handoff Notes

- Document key prefixes, TTLs, and concurrency limits.

## Stop Conditions

- Stop if task 01's storage policy is incomplete.
- Stop if a deferral would have no owning task file.

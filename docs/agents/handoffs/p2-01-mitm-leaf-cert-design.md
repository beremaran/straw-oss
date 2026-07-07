# Handoff

Task: `docs/tasks/p2/01-mitm-leaf-cert-design.md`

## Changed

- Created `docs/planning/c-mitm-leaf-certificate-design.md`: MITM leaf-certificate storage, encryption, TTL, rotation,
  cache-miss coalescing, and unique-SNI flood-control design.
- Modified `docs/planning/INDEX.md`: linked the new MITM leaf-certificate appendix.
- Modified `docs/planning/30-testing-matrix.md`: added MITM certificate test rows required before implementation.
- Modified `docs/tasks/p2/01-mitm-leaf-cert-design.md`: marked task status and steps done.
- Modified `docs/tasks/p2.md`: marked P2 task 01 done.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report:

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Certificate private-key handling is fully decided before code work starts. | VERIFIED | `docs/planning/c-mitm-leaf-certificate-design.md:17` defines KMS-backed shared cache; `docs/planning/c-mitm-leaf-certificate-design.md:19` defines generated bundle, encryption, scoping, and decrypt permission; `docs/planning/c-mitm-leaf-certificate-design.md:25` rejects alternatives. | Documentation/spec review; `make check` |
| CPU/flood controls are explicit. | VERIFIED | `docs/planning/c-mitm-leaf-certificate-design.md:59` lists local singleflight, Redis lock, bounded generation concurrency, per-tenant limits, and global unique-SNI limits; `docs/planning/c-mitm-leaf-certificate-design.md:67` defines lock TTL and rejection behavior. | Documentation/spec review; future required tests listed at `docs/planning/c-mitm-leaf-certificate-design.md:83` and mirrored in `docs/planning/30-testing-matrix.md:53`; `make check` |
| No production code is changed. | VERIFIED | Changed files are planning/task/handoff docs only. | Diff/content review; `make check` |

Verifier gap noted before this file existed: the handoff note itself was missing. This file closes that gap.

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Resolved KMS-backed shared-cache private-key policy | implemented | `docs/planning/32-open-decisions.md:38`; `docs/planning/c-mitm-leaf-certificate-design.md:17` |
| Rejected key-storage options | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:25` |
| Leaf public cert bytes cacheable | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:34` |
| Leaf private keys stored only encrypted | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:34`; `docs/planning/c-mitm-leaf-certificate-design.md:42` |
| Redis cache/lock TTL discipline | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:38`; `docs/planning/c-mitm-leaf-certificate-design.md:67` |
| Optional disk cache encrypted when private key included | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:39` |
| Optional shared object-storage cache encrypted and scoped | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:40` |
| TTL no longer than certificate validity days; default 30 days | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:49` |
| Control-only access to generated bundles | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:10`; `docs/planning/c-mitm-leaf-certificate-design.md:23` |
| Local singleflight | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:61` |
| Redis distributed lock across instances | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:62` |
| Bounded generation concurrency | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:63` |
| Per-tenant and global unique-SNI flood limits | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:64`; `docs/planning/c-mitm-leaf-certificate-design.md:70` |
| Rotation, cache-miss, encrypted-at-rest tests | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:73`; `docs/planning/30-testing-matrix.md:53` |
| Control CPU saturation risk | implemented | `docs/planning/c-mitm-leaf-certificate-design.md:57`; `docs/planning/c-mitm-leaf-certificate-design.md:70` |

## Verification

```sh
make check
```

Result: passed.

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped (documentation-only changes, no runtime logic altered).

## Reviewer Start Points

- `docs/planning/c-mitm-leaf-certificate-design.md`
- `docs/planning/32-open-decisions.md`
- `docs/planning/30-testing-matrix.md`

## Remaining Work

- MITM ingress implementation is owned by `docs/tasks/p2/02-mitm-ingress.md`.
- MITM CA management is owned by `docs/tasks/p2/03-mitm-ca-management.md`.
- MITM leaf bundle KMS provider/config is owned by `docs/tasks/p2/19-mitm-leaf-bundle-kms-provider.md` and must be
  completed before task 04.
- MITM leaf certificate cache implementation is owned by `docs/tasks/p2/04-mitm-leaf-cert-cache.md` after task 19.

## Blockers

- None.

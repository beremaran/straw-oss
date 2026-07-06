# Handoff

Task: `docs/tasks/p1/16-upstream-connection-pooling.md`

## Changed

- `docs/planning/b-upstream-connection-pooling.md`: added the P1 default-off upstream connection-pooling spec, config keys, pool boundaries, SSRF invariant, eviction/shutdown rules, observability limits, and implementation test rows.
- `docs/planning/30-testing-matrix.md`: references the required P1 pooling test rows before implementation ships.
- `docs/planning/INDEX.md`: links the existing and new Appendix B planning docs.
- `docs/planning/24-static-configuration.md`: updates a stale credential-schema ownership note to point at P1 task 22.
- `docs/tasks/p1.md` and `docs/tasks/p1/16-upstream-connection-pooling.md`: marked task 16 complete after verification.
- `docs/tasks/p1/26-upstream-connection-pooling-implementation.md`: adds the owning follow-up task for the spec's
  implementation work.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Pooling remains default-off and explicitly tested before code work starts. | VERIFIED | `docs/planning/b-upstream-connection-pooling.md:11`; `docs/planning/b-upstream-connection-pooling.md:16`; `docs/planning/b-upstream-connection-pooling.md:21`; `docs/planning/b-upstream-connection-pooling.md:77`; `docs/tasks/p1/26-upstream-connection-pooling-implementation.md:47` | Documentation/spec review plus `make check` |
| The design does not rely on cross-request reuse for correctness. | VERIFIED | `docs/planning/b-upstream-connection-pooling.md:7`; `docs/planning/b-upstream-connection-pooling.md:73`; `docs/planning/16-egress-execution.md:16` | Documentation/spec review plus `make check` |
| The SSRF invariant is preserved. | VERIFIED | `docs/planning/b-upstream-connection-pooling.md:40`; `docs/planning/b-upstream-connection-pooling.md:44`; `docs/planning/b-upstream-connection-pooling.md:49`; `docs/planning/27-security-controls.md:100` | Documentation/spec review plus `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P0 disables outbound HTTP/2 and upstream keep-alives by default. | implemented | Default-off spec preserves P0 behavior in `docs/planning/b-upstream-connection-pooling.md:9`; existing code remains unchanged in `internal/egress/executor.go:118`. |
| P0 must not rely on cross-request upstream connection reuse for correctness or performance claims. | implemented | Pooling is optional and not required for correctness in `docs/planning/b-upstream-connection-pooling.md:7`; failure fallback in `docs/planning/b-upstream-connection-pooling.md:62`. |
| P1 may add optional upstream connection pooling only if explicitly specified and tested. | implemented | Spec and test rows in `docs/planning/b-upstream-connection-pooling.md:1`; `docs/planning/b-upstream-connection-pooling.md:71`; matrix reference in `docs/planning/30-testing-matrix.md:45`; implementation owner in `docs/tasks/p1/26-upstream-connection-pooling-implementation.md:1`. |
| Upstream keep-alives may be re-enabled only behind an explicit tested feature flag in a later phase. | implemented | Feature flag and default-off behavior in `docs/planning/b-upstream-connection-pooling.md:9`. |
| Resolver, destination-policy validator, and dialer are one unit for direct local resolution. | implemented | SSRF invariant steps in `docs/planning/b-upstream-connection-pooling.md:35`. |
| Egress must connect only to an IP that passed policy validation for the current request. | implemented | Reuse limited to current validated IP set in `docs/planning/b-upstream-connection-pooling.md:39`; stale IP eviction in `docs/planning/b-upstream-connection-pooling.md:55`. |
| The HTTP/TLS library must not perform an independent second resolution. | implemented | Explicit second-resolution ban in `docs/planning/b-upstream-connection-pooling.md:44`; test row in `docs/planning/b-upstream-connection-pooling.md:79`. |
| SNI, certificate verification, and Host remain bound to the original hostname. | implemented | Original-host binding in `docs/planning/b-upstream-connection-pooling.md:44`. |
| Destination deny rules and metadata/log redaction remain enforced. | implemented | Validation before reuse in `docs/planning/b-upstream-connection-pooling.md:35`; bounded observability labels in `docs/planning/b-upstream-connection-pooling.md:58`. |
| Section 30 requires connection-pooling test rows before the feature ships. | implemented | Matrix requirement in `docs/planning/30-testing-matrix.md:45`; detailed rows in `docs/planning/b-upstream-connection-pooling.md:71`. |
| Existing static config has no pooling keys. | implemented | New proposed keys documented in `docs/planning/b-upstream-connection-pooling.md:9`; no runtime config code changed because task is spec-only. |

## Verification

```sh
make check
```

Result:

- `make check`: passed.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because this task is a documentation/spec task and does not change the runtime request path.

## Reviewer Start Points

- `docs/planning/b-upstream-connection-pooling.md`
- `docs/planning/30-testing-matrix.md`
- `docs/tasks/p1/16-upstream-connection-pooling.md`

## Remaining Work

- Pooling implementation is owned by `docs/tasks/p1/26-upstream-connection-pooling-implementation.md`.

## Blockers

- None.

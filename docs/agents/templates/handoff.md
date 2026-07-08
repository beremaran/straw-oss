# Handoff

Feature: `specs/<feature>/`
Task: `specs/<feature>/tasks.md#TNNN`

## Changed

- Files changed and why.

## Acceptance Criteria Verdicts

Filled from `/speckit-analyze` or a fresh verifier's report, not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| ... | VERIFIED / NOT MET | `path/file.go:123` | `TestName` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the SpecKit feature's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| ... | implemented / already existed / out of scope | `file:line` or `specs/<feature>/tasks.md#TNNN` |

## Verification

```sh
make check
```

Result:

- Postgres-backed tests: ran against `straw_test` / not exercised (diff does not touch Postgres surfaces).
- Live compose verification: request driven through the stack / skipped because ...

## Reviewer Start Points

- `path/to/file.go`

## Remaining Work

- None. (Only valid when nothing is faked, stubbed, or deferred. Every deferral names an open SpecKit task.)

## Blockers

- None. (State here if the work is uncommitted.)

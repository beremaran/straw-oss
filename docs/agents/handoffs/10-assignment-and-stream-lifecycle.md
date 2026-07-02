# Handoff

Task: `docs/tasks/p0/10-assignment-and-stream-lifecycle.md`

## Changed

- `internal/control/lifecycle.go` (new): assignment lifecycle state machine for accept/reject, ack timeout, `RequestStart` boundary, fallback gating, terminal handling, cancellation, admin-cancel authorization, and executor error-code validation against the P0 emit set.
- `internal/control/lifecycle_test.go` (new): focused checks for assignment reject/accept, fallback boundary, cancellation semantics, admin-cancel scoping, executor error mapping, and ack timeout bounding.
- `internal/egress/assignment.go` (new): executor-side `AssignRequest` admission decision plus `FakeExecutor` frame scripting for lifecycle tests.
- `internal/egress/assignment_test.go` (new): admission precedence and scripted success-frame sequencing.
- `internal/natsx/stream.go` (new): stream-frame ordering, attempt, offset, credit, and idle-timeout validation.
- `internal/natsx/stream_test.go` (new): sequence gap, duplicate, terminal, offset mismatch, credit exhaustion, and idle-expiry checks.

## Verification

```sh
go test ./internal/control ./internal/egress ./internal/natsx
make check
```

Result:

- Focused package tests pass.
- `make check` fails in repo-wide linting with pre-existing issues outside this task scope, including `cyclop`, `err113`, `errcheck`, `errchkjson`, `errorlint`, `exhaustive`, `funcorder`, `funlen`, `goconst`, `mnd`, `nlreturn`, `noctx`, `noinlineerr`, `nonamedreturns`, `revive`, `wsl_v5`, and related findings across many files. The new lifecycle files are not the source of that broader failure.

## Reviewer Start Points

- `internal/control/lifecycle.go`
- `internal/natsx/stream.go`
- `internal/egress/assignment.go`

## Remaining Work

- Resolve the repository-wide lint backlog so `make check` can pass.
- Re-run `make check`, then update `docs/tasks/p0/10-assignment-and-stream-lifecycle.md` to `done` only after the repo gate is green.

## Blockers

- Repository-wide lint failures unrelated to this task.

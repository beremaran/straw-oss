# MB-022: Final Acceptance And Compatibility Pass

Status: not-started
Phase: cross-cutting
Depends on: MB-021
Search tags: final acceptance, regression, legacy routes, management backend spec

## Objective

Prove the Management Backend Specification is fully implemented and existing behavior still works.

## Scope

- Run the spec acceptance checklist end to end.
- Run unit, integration, security, and contract tests.
- Verify legacy-token compatibility and user-session auth.
- Verify Management UI backend dependency items are all supported.
- Update tracker statuses and close any missed tasks.

## Repo Touchpoints

- `docs/management-backend-spec.md`
- `docs/management-ui-spec.md`
- `docs/management-backend-tasks/tracker.md`
- `test/integration/*`
- `test/security/*`
- `test/contract/*`
- `Makefile`

## Implementation Tasks

- [ ] Convert every acceptance checklist item in the spec into a concrete verification note.
- [ ] Run `go test ./...`.
- [ ] Run any documented security, integration, load, and contract checks that apply.
- [ ] Verify no new management route bypasses auth/RBAC.
- [ ] Verify no read API returns stored secrets.
- [ ] Update tracker and task statuses to reflect completed work.

## Done Criteria

- [ ] All checklist items in `docs/management-backend-spec.md` pass.
- [ ] Existing Management API routes continue to pass their current tests.
- [ ] New Management UI backend dependencies are implemented.
- [ ] Any remaining deferred work is documented as a new explicit task.

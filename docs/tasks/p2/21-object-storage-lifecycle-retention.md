# 21 - Object Storage Lifecycle Retention

Status: done

## Objective

Enforce the Section 18 retention/lifecycle backstop that expires orphaned body objects, so objects left behind by a
Control crash (upload succeeded, explicit abort/DELETE never ran) do not accumulate indefinitely in the body bucket.

## Background

Task 06 (`docs/tasks/p2/06-object-storage-foundation.md`) built the object-storage client and deferred "lifecycle
cleanup" to the BodyRef tasks. Task 07 (`docs/tasks/p2/07-bodyref-request-body-flow.md`) implemented the
request-flow's *explicit* cleanup — Control DELETEs the uploaded object on cancellation or request-stream publish
failure, and an in-flight PUT is aborted by context cancellation — but did not implement the bucket-level lifecycle
rule that `docs/planning/18-large-body-transport-p2.md` step 9 ("Lifecycle rules clean up any orphaned objects")
requires as the backstop for the crash path. The objectstore client exposes `Retention()` (1–3 days) precisely so a
provisioning step can apply it; nothing applies it yet. This backstop is shared by the request (07), response (08),
and payload-capture (11) flows.

## Required Planning Docs

- `docs/planning/18-large-body-transport-p2.md` (Retention; S3 flows step 9)
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 06 completed.

## Out of Scope

- Do not change the request/response BodyRef upload or download flows.
- Do not change the explicit per-object DELETE/abort already implemented in tasks 07/08.

## Steps

- [x] Read the required planning docs.
- [x] Decide and document whether the lifecycle rule is applied by Control at startup (a signed
      `PutBucketLifecycleConfiguration` using the configured `Retention()`) or provisioned as operator infrastructure
      (documented bucket setup for compose/MinIO and production), then implement the chosen path.
- [x] Ensure the rule expires objects under the body-object prefix at the configured retention (default 1 day, max 3).
- [x] Provide/verify object-storage bucket provisioning for the local compose stack so the flow is observable live.
- [x] Add tests proving the lifecycle rule is applied with the configured expiration, and that retention stays within
      the 1–3 day bound.
- [x] Run `make check`.
- [x] Write a handoff note.

## Acceptance Criteria

- Orphaned body objects (upload completed, no explicit DELETE) are expired by a bucket lifecycle rule at the configured
  retention.
- Retention honored is the configured value, clamped to 1–3 days.
- The mechanism (app-applied vs operator-provisioned) is documented for both compose and production.

## Handoff Notes

- Document where the lifecycle rule lives and how an operator overrides retention.

## Stop Conditions

- Stop if applying the rule requires a new dependency the standard library / existing objectstore signing cannot cover
  without justification.
- Stop if a deferral would have no owning task file.

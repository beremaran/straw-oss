# 10 - Payload Capture Engine

Status: done

## Objective

Implement the non-mutating payload capture tee with storage-only redaction and bounded capture decisions.

## Required Planning Docs

- `docs/planning/19-payload-capture-p2.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Task 09 completed.

## Out of Scope

- Do not mutate forwarded request or response bytes.
- Do not add body regex/JSONPath redaction.
- Do not decompress bodies unless a later task explicitly owns it.

## Expected Files

- Create or modify: payload capture engine.
- Test: payload capture engine tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement capture decisions `NONE`, `METADATA_ONLY`, `HEADERS`, `BODY_TRUNCATED`, and bounded `BODY_FULL`.
- [x] Tee request/response data without mutating forwarded bytes.
- [x] Apply header redaction to stored copies.
- [x] Implement raw-body truncation for stored copies.
- [x] Treat compressed bodies as raw compressed bytes or metadata-only according to policy.
- [x] Enforce capture size limits and disabled-by-default behavior.
- [x] Add tests for every capture decision, non-mutation, redaction, truncation, compressed body policy, and limit
      enforcement.
- [x] Run focused capture engine tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused payload capture engine tests.
- `make check`

## Acceptance Criteria

- Capture never changes forwarded traffic.
- Full capture is still bounded.
- Baseline P2 supports header redaction and raw-body truncation only.

## Handoff Notes

- Compression handling checks the "Content-Encoding" headers. If compression is detected and not allowed by CaptureOptions, the bodies are dropped.
- Body regex and JSONPath redaction are out of scope for Phase 2 baseline.

## Stop Conditions

- Stop before adding live body mutation.
- Stop if a deferral would have no owning task file.

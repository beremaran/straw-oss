# 24 - Streaming Credit Replenishment

Status: not started

## Objective

Make the e2c response path actually credit-governed: the egress executor streams upstream response bodies as
multiple `DataFrame`s bounded by granted download credit (instead of one buffered frame), and Control replenishes
download credit with `CreditFrame`s on the c2e subject as it consumes buffered bytes, per the credit rules in
`docs/planning/12-nats-protocol.md`.

## Context (gap being closed)

`docs/agents/handoffs/24-control-request-dispatch-pipeline.md` flagged: "`CreditFrame` replenishment on the c2e
channel is not sent ... the egress executor publishes the full response in one shot (single `DataFrame`) ... Real
streaming credit replenishment is a P1 concern" — with no owning task named. Verified in current code
(2026-07-06 sweep):

- `internal/egress/loop.go:363-367` — received `CreditFrame`s are deliberately ignored; the comment says P0
  executes the whole response in one pass rather than chunking by credit.
- `internal/control/dispatcher.go` — grants only the initial download credit via `AssignRequest`
  (`InitialDownloadCreditBytes`, line ~681); no replenishment `CreditFrame` is ever published.

P1 task 03 (raw streaming response path) owns Control-side client-facing backpressure but its Expected Files are
Control-only; P1 task 05 (raw CONNECT tunnel) *assumes* "the existing c2e/e2c credit protocol" works. Neither owns
the egress-side chunked send or Control's replenishment — this task does, and both depend on it.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md` ("Backpressure and Credit Semantics", lines ~127-157; frame sequencing)
- `docs/planning/09-request-lifecycle.md` (response streaming lifecycle)

## Prerequisites

- None beyond P0 (protocol frames, dispatcher, and egress loop all exist). Blocks P1 tasks 03 and 05.

## Out of Scope

- Do not build the client-facing raw response writer (P1 task 03).
- Do not implement CONNECT tunneling (P1 task 05).
- Do not change upload-direction (c2e request body) credit behavior beyond what symmetry requires.
- Do not change the P0 REST JSON envelope or its inline body limit.

## Expected Files

- Modify: `internal/egress/loop.go` (track download credit from `AssignRequest` + received `CreditFrame`s; emit
  response body as sequenced `DataFrame`s of at most `max_frame_data_bytes`, pausing at zero credit).
- Modify: `internal/control/dispatcher.go` (publish sequenced `CreditFrame`s on c2e as buffered response bytes are
  consumed/released, respecting max in-flight download bytes).
- Modify: `internal/natsx/stream.go` if the shared validator needs credit-accounting helpers.
- Test: egress splits a body larger than the frame limit into multiple frames; egress stops sending at zero credit
  and resumes on a `CreditFrame`; Control replenishes as it drains; end-to-end dispatcher test streams a body
  larger than the initial download credit.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement credit-bounded chunked sending in the egress executor loop (`cmd/egress` binary path).
- [ ] Implement download-credit replenishment publication in the Control dispatcher (`cmd/control` binary path).
- [ ] Enforce `DataFrame` sequencing and the terminal-frames-do-not-consume-credit rule on both sides.
- [ ] Update the stale `loop.go` comment and the handoff-24 flag to point here / reflect the new behavior.
- [ ] Add the tests listed in Expected Files.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/egress ./internal/control ./internal/natsx`
- `make check`

## Acceptance Criteria

- A response body larger than initial download credit completes: egress sends multiple `DataFrame`s, blocks at zero
  credit, and resumes only after Control's `CreditFrame` (proven by tests on both sides plus one round-trip test).
- Control never lets un-replenished buffered bytes exceed `max_inflight_download_bytes` (proven by test).
- The "one shot" comment at `internal/egress/loop.go:363-367` is gone (grep for "one pass" comes back empty), and
  handoff 24's CreditFrame bullet names this task as owner.

## Handoff Notes

- Record the replenishment policy chosen (when/how much credit is granted back) and cite the planning/12 rule.

## Stop Conditions

- Stop if planning/12 credit rules are ambiguous for a case the tests need (e.g. replenishment granularity) — ask.
- Stop if a deferral would have no owning task file.

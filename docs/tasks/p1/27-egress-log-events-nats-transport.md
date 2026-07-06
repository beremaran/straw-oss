# 27 - Egress Log Events NATS Transport

Status: not started

## Objective

Ship Egress worker structured logs to the canonical ClickHouse `log_events` table through Control: Egress emits
bounded, non-blocking log telemetry over Core NATS, and Control receives it and enqueues rows through the existing
Control-owned `log_events` writer.

## Context (gap being closed)

P1 task 20 was split on 2026-07-06 after review found its original `cmd/egress/main.go` ClickHouse-wiring step
conflicted with `docs/planning/04-canonical-architecture.md`: Control owns observability aggregation, and executors are
not allowed to query ClickHouse. Current code proves the Egress half is not implemented:

- `cmd/egress/main.go:69` sets a local stdout JSON logger with `service=egress`; it has no log transport writer.
- `api/proto/straw/v1/straw.proto:116-124` defines NATS envelope payloads for registration, heartbeat, assignment,
  and request streams only; there is no log-event payload.
- `docs/planning/12-nats-protocol.md:52-58` defines no canonical log telemetry subject.

Task 20 owns Control's local `slog` tee into ClickHouse. This task owns the Egress-to-Control transport for the same
canonical sink. ClickHouse remains the only canonical log sink; do not add Loki.

## Required Planning Docs

- `docs/planning/04-canonical-architecture.md` (Control observability aggregation; no executor ClickHouse access)
- `docs/planning/12-nats-protocol.md` (Envelope, Core NATS subjects, no JSON inside NATS, ACLs)
- `docs/planning/22-canonical-clickhouse-schema.md` (`log_events` row shape and bounded async writes)
- `docs/planning/23-observability.md` (Egress telemetry over NATS option; log fields)
- `docs/planning/27-security-controls.md` (log redaction and NATS subject-token rules)
- `docs/planning/21-state-and-storage.md` (ClickHouse role and log/metadata redaction boundary)

## Prerequisites

- P1 task 20 completed (Control `log_events` writer and `slog` row mapping exist).
- P0 task 37 completed (both binaries emit structured `slog` JSON).
- P0 tasks 16 and 17 completed (Control and Egress already have live NATS connections for registration/heartbeat).

## Out of Scope

- Do not add ClickHouse config, credentials, or direct ClickHouse writes to Egress.
- Do not add Loki, OpenTelemetry, Promtail, or any other canonical log sink.
- Do not build log search/read APIs.
- Do not capture request or response payloads.
- Do not introduce JetStream, durable log queues, or replay semantics.

## Expected Files

- Modify: `api/proto/straw/v1/straw.proto` (add protobuf log-event payload/message fields; keep NATS binary
  protobuf-only).
- Modify: generated protobuf files under `api/proto/straw/v1` as required by the repo's generation workflow.
- Modify: `docs/planning/12-nats-protocol.md` (add the canonical Egress-to-Control log telemetry subject and ACL row).
- Modify: `internal/natsx` (subject helper and validation for the log telemetry subject).
- Modify: `internal/logging` if the existing tee needs a reusable NATS publisher adapter.
- Modify: `cmd/egress/main.go` (install a bounded, non-blocking NATS log publisher after NATS connects).
- Modify: `cmd/control/main.go` and/or `internal/control` (subscribe to Egress log telemetry and enqueue received rows
  through the existing `LogEventWriter`).
- Test: NATS subject/protobuf validation, Egress publisher drop/outage behavior, Control receiver row mapping,
  redaction, and no Egress direct-ClickHouse config.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the protobuf log-event message and the canonical Egress-to-Control NATS subject.
- [ ] Add NATS subject helpers and update the protocol/ACL documentation.
- [ ] Implement a bounded Egress log publisher that never blocks the logging caller and drops oldest non-critical log
      events on overflow or NATS outage.
- [ ] Wire `cmd/egress` to publish structured log events over NATS after connection, without adding ClickHouse config.
- [ ] Wire Control to subscribe to the log telemetry subject and enqueue rows through the existing `LogEventWriter`.
- [ ] Verify redaction: no secret material, NATS subjects, credentials, private keys, signed URLs, or upstream proxy
      credentials can reach `log_events`.
- [ ] Add the tests listed in Expected Files.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./api/... ./internal/natsx ./internal/logging ./internal/control ./internal/egress ./cmd/...`
- `make check`

## Acceptance Criteria

- An Egress `slog` record is published over the canonical protobuf NATS log telemetry path, received by Control, and
  enqueued as a `log_events` row with the `docs/planning/22` shape, proven by tests.
- Egress has no ClickHouse config surface and no direct ClickHouse writer (prove by config/code test or grep).
- NATS outage, missing Control subscribers, or a full Egress log queue never blocks or fails the logging caller; overflow
  drops are bounded and observable.
- Redaction invariants hold for Egress log records before they can reach ClickHouse.
- `docs/planning/12-nats-protocol.md` documents the subject, payload, and ACLs used by the implementation.

## Handoff Notes

- Document the NATS subject, protobuf message, queue bounds, drop policy, redaction behavior, and any live verification
  performed.

## Stop Conditions

- Stop if the protocol change requires a durable/replayed log queue; that would contradict the Core NATS transient
  boundary.
- Stop if redaction for an Egress field is ambiguous in the planning docs; ask before shipping.
- Stop if a deferral would have no owning task file.

# MB-013: Endpoint Control Plane And Worker Commands

Status: not-started
Phase: 3
Depends on: MB-011, MB-012
Search tags: endpoint_control, NATS, worker subscriber, drain, undrain, restart, ack

## Objective

Publish endpoint control commands and teach workers to acknowledge and execute them.

## Scope

- Declare `endpoint_control` stream and subjects from the spec.
- Publish command payloads for drain, undrain, restart, disable, and enable.
- Subscribe workers to `endpoint.control.<ENDPOINT_ID>`.
- Publish acknowledgement and status updates to `endpoint.control.ack.<command_id>`.
- Update command status from acknowledgements and timeouts.

## Repo Touchpoints

- `internal/server/admin/server.go`
- `internal/server/admin/handlers/endpoints.go`
- `internal/service/endpoint/*`
- `pkg/endpoint/worker.go`
- `pkg/endpoint/heartbeat.go`
- `pkg/broker/*`
- `internal/endpoint/update/*`

## Implementation Tasks

- [ ] Add command payload types shared by relay and worker.
- [ ] Add broker stream declaration for endpoint control.
- [ ] Publish commands when endpoint control handlers accept work.
- [ ] Add worker subscription and handlers for required commands.
- [ ] Add acknowledgement consumer that updates `endpoint_commands`.
- [ ] Add timeout handling for commands without acknowledgement or completion.

## Done Criteria

- [ ] Successful control API calls create command records and publish command messages.
- [ ] Worker publishes acknowledgement before long-running operations.
- [ ] Command status updates from worker acknowledgement messages.
- [ ] Restart uses existing worker update/restart primitives where possible.

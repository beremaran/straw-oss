# Private `straw.v1` protocol inventory

Archived historical evidence; not current installation or release guidance.

This inventory records the pre-extraction classification and product-boundary audit required by the private split.

## Current files

| File | Classification | Permanent owner |
| --- | --- | --- |
| `straw.proto` | Normative language-neutral schema | `straw-protos` |
| `straw.pb.go` | Generated Go binding; never hand-edited | `straw-protos-go` |
| `validate.go` | Hand-written Go protocol validation | `straw-protos-go` |
| `registration_sign.go` | Hand-written Go signing and verification | `straw-protos-go` |
| `contract_test.go` | Go binding and validation contract tests | `straw-protos-go` |
| `registration_sign_test.go` | Go signing tests | `straw-protos-go` |
| `fixture_test.go` | Go language-neutral fixture runner | `straw-protos-go` |
| `README.md` | Package-specific documentation; adapted per repository | `straw-protos` and `straw-protos-go` |

Python generated bindings and stubs currently under `python/straw/proto` move to `straw-protos-python`; the equivalent
signing implementation and fixture tests move there as hand-written protocol helpers. SDK worker machinery and its
tests move to the SDK repositories, not to protocol repositories.

## Schema surface

The audited enums are `ErrorCode`, `ErrorCategory`, `TimeoutType`, `AssignAckCode`, `RequestMode`, `WorkerHealth`,
`SniHostMismatchPolicy`, `RedirectPolicy`, and `DestinationResolutionMode`.

The audited messages are `Envelope`, `LogEvent`, `RegisterRequest` and `PoolRef`, `RegisterAck`, `HeartbeatRequest`,
`HeartbeatAck`, `AssignRequest`, `AssignAck`, `StreamFrame`, `Header`, `DataFrame`, `CreditFrame`, `ErrorFrame`,
`EndFrame`, `CancelFrame`, `CancelledFrame`, `TrailersFrame`, `InjectionOperation`, `DestinationPolicy`,
`BodyRefFrame`, `S3BodyRef`, `DirectStreamRef`, `RequestStart`, `OutboundStartFrame`, `ResponseStart`, and
`ErrorResponse`.

The audited NATS subjects are registration (`straw.v1.control.register`), heartbeat (`straw.v1.control.heartbeat`),
session assignment, bidirectional request streams, and runtime configuration snapshot/ack subjects. Runtime
configuration subjects remain runtime-owned; canonical worker subjects belong to SDK implementations and their
conformance tests.

## Cleanup decision

The obsolete tenant, permission, quota, rate-limit, and assignment auth-scope concepts are removed before the first
tag. Their enum numbers and names are reserved. Active deployment-scoped authentication remains. Wire fields named
`tenant_id` become `deployment_id` at their existing field numbers. Body-reference keys become deployment-scoped.
Pools, routing, capacity, destination policy, receipts, cancellation, credit/backpressure, and worker lifecycle are
active deployment concepts and remain.

The `straw.v1` namespace is private and compatibility-governed only after a separately approved public release. The
language-neutral fixtures define the present private conformance baseline without promising compatibility with older
private `v0.x` tags.

# egress-static

A minimal, vendor-neutral example Egress worker built entirely on the public
`sdk/egress` package and the Go standard library. It answers every assignment
with the same fixed HTTP status and body — it never resolves a hostname or
opens an upstream connection. It exists to prove that a third party can
implement a custom Egress node (including one that forwards to a commercial
provider) using only the public SDK, without importing any `internal/*`
package (`docs/planning/05-component-boundaries.md`, "Egress SDK and Custom
Egress Implementations").

## Running it

The worker needs a NATS server it can reach and a Control instance that has a
`worker_credentials` row whose stored ed25519 public key matches the key this
worker registers with.

```sh
go run ./examples/egress-static \
  -nats-servers nats://127.0.0.1:4222 \
  -worker-id wrk_egress_static_example \
  -credential-id <credential-id-configured-in-control> \
  -max-concurrency 4 \
  -status 200 \
  -body "static-response\n"
```

On startup the worker logs its base64-encoded ed25519 public key
(`public_key_b64`). Register that key as the credential's public key in
Control before the worker can complete registration (see
`docs/planning/27-security-controls.md` for the registration/heartbeat
protocol this SDK speaks).

By default the worker generates a fresh ed25519 key on every start, which is
fine for local experimentation but means the worker cannot pass Control's
registration signature check without a fresh matching credential. For a
persistent identity, set `STRAW_EGRESS_STATIC_PRIVATE_KEY_B64` to a
base64-standard-encoded ed25519 seed (32 bytes) or full private key (64
bytes), matching the convention `cmd/egress` uses for its own worker key.

## Operator obligations

`docs/planning/05-component-boundaries.md` states that a custom Egress
implementation is operator-configured only, and the operator assumes the
same executor-side obligations the official worker enforces. Two apply to any
implementation that reaches a real upstream (this static example does not,
since it never dials out — see below for what changes if you extend it):

1. **Equivalent destination-policy enforcement.** A custom implementation
   that resolves or connects to destinations internally (executor-delegated
   mode, `DESTINATION_RESOLUTION_EXECUTOR_DELEGATED`) must apply the same
   deny-list checks Egress applies in direct-resolution mode: block private,
   loopback, link-local, multicast, cloud metadata-service, and documentation
   IP ranges, plus any tenant-configured deny rules, before ever connecting
   (`docs/planning/16-egress-execution.md` "Executor-delegated mode";
   `docs/planning/27-security-controls.md` "Executor-delegated resolution").
   This example has no such obligation today because `staticExecutor.Execute`
   never resolves or dials anywhere; the moment an implementation forwards a
   request to any real destination (a provider, an upstream proxy, or the
   target host directly), this check becomes mandatory.

2. **Constrained, public-safe execution facts.** Whatever the implementation
   reports back to Control (via `ErrorFrame` details or logs) must stay
   within the same redaction rules the official worker follows: no worker or
   session IDs, no upstream credentials, proxy addresses, or private keys in
   any field a tenant can read (`docs/planning/27-security-controls.md`
   "Metadata and Log Redaction"). This example's only reported fact is a
   fixed status/body pair, which carries no sensitive information by
   construction.

There is no marketplace discovery, provider billing reconciliation, or
provider account provisioning in Straw — a provider integration is just a
custom Egress implementation an operator runs and is responsible for
(`docs/planning/05-component-boundaries.md`).

## SDK friction notes

None found. `sdk/egress` exposes everything this example needed:
`Identity`, `Capabilities`, `Register`, `Run`, `AssignmentFactory`,
`NewWorker`/`WorkerOptions`, and the `Executor` interface. No `internal/*`
import was required to build a working worker or to drive one assignment
through the SDK's real NATS wire protocol in a test.

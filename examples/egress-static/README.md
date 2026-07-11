# Static Egress example

This is a minimal custom worker built only with the public `sdk/egress` package and Go's standard library. It returns
the same status and body for every assignment and never resolves a hostname or opens an upstream connection.

Start the normal local stack, stop its official worker, then run the example:

```sh
docker compose -f deploy/local/docker-compose.yml stop egress
go run ./examples/egress-static \
  -nats-servers nats://127.0.0.1:4222 \
  -worker-id egress-static-1 \
  -max-concurrency 4 \
  -status 200 \
  -body "static-response\n"
```

Send the request from the [quickstart](../../docs/public/quickstart.md). The upstream URL is still validated by
Control, but this example returns its fixed response instead of dialing it.

The protocol retains a `credential-id` field for wire compatibility; the single-deployment runtime does not require
database provisioning. The example generates an ephemeral Ed25519 identity on startup. Set
`STRAW_EGRESS_STATIC_PRIVATE_KEY_B64` to a base64-encoded 32-byte seed or 64-byte private key when a stable custom
worker identity is useful.

## Extending the example

A custom worker that connects to real destinations becomes responsible for the same safety properties as the
official worker: deadline handling, capacity, cancellation, resolved-IP checks, TLS verification, stream framing,
and avoiding credentials or private network details in returned errors. Start with the official worker unless the
executor itself is the feature you need to replace.

See the [worker guide](../../docs/public/egress_worker.md) and `sdk/egress` package documentation.

# Local development stack

The default Compose stack runs only NATS, Straw Control, and one Egress worker.

From the repository root:

```sh
make dev
```

No credentials or provisioning are required. Send a request with:

```sh
curl -sS \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://example.com"}' \
  http://localhost:8080/api/v1/requests
```

Useful commands:

```sh
make dev-status
make dev-logs
make dev-down
make dev-reset
```

If a default host port is occupied, override it for the command:

```sh
STRAW_NATS_PORT=14222 \
STRAW_NATS_MONITOR_PORT=18222 \
STRAW_CONTROL_API_PORT=18080 \
STRAW_CONTROL_METRICS_PORT=19090 \
make dev
```

These variables change host port mappings only; containers continue to use the standard internal ports.

## Runtime-administration example

```sh
make dev-admin
open http://localhost:8080/admin/
```

The development-only admin token defaults to `local-admin`. Set `STRAW_ADMIN_TOKEN` to override it. This opt-in
overlay enables durable JetStream storage; see `docs/public/runtime-administration.md` for API examples and recovery.

# Straw

Straw is a small HTTP control service that proxies requests through a configured egress worker over NATS.

```text
client -> control /v1/request -> NATS tasks.<egress>.tasks -> egress -> NATS results.<request> -> client
```

## Run Locally

Start NATS:

```bash
docker compose -f docker/docker-compose.dev.yml up -d nats
```

Run an egress:

```bash
EGRESS_ID=dev-worker-01 \
NATS_URL=nats://localhost:4222 \
go run ./cmd/egress
```

Run the control:

```bash
CONTROL_EGRESS_ID=dev-worker-01 \
NATS_URL=nats://localhost:4222 \
ALLOW_PRIVATE_IPS=true \
go run ./cmd/control
```

Proxy a request:

```bash
curl -X POST http://localhost:8080/v1/request \
  -H "Content-Type: application/json" \
  -d '{"method":"GET","url":"https://httpbin.org/get"}'
```

## Configuration

Control:

- `CONTROL_EGRESS_ID`: egress worker ID to target. Falls back to `EGRESS_ID`.
- `NATS_URL`, `NATS_TOKEN`: NATS connection.
- `HTTP_PORT`: control listen port. Default `8080`.
- `RESULT_TIMEOUT`: egress response timeout. Default `30s`.
- `MAX_BODY_SIZE`: control request body limit. Default `2M`.
- `MAX_CONCURRENT_REQUESTS`: control concurrency cap. Default `50`.
- `ALLOW_PRIVATE_IPS`: allow private target URLs. Default `false`.

Egress:

- `EGRESS_ID`: worker ID. The control targets this value.
- `NATS_URL`, `NATS_TOKEN`: NATS connection.
- `CONCURRENCY_LIMIT`: worker task concurrency. Default `25`.

## Development

```bash
make build
make test
make lint
```

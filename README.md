# Straw

Straw is a small HTTP relay that proxies requests through a configured endpoint worker over NATS.

```text
client -> relay /v1/request -> NATS tasks.<endpoint>.tasks -> endpoint -> NATS results.<request> -> client
```

## Run Locally

Start NATS:

```bash
docker compose -f docker/docker-compose.dev.yml up -d nats
```

Run an endpoint:

```bash
ENDPOINT_ID=dev-worker-01 \
NATS_URL=nats://localhost:4222 \
HMAC_SECRET=dev-secret-change-me \
go run ./cmd/endpoint
```

Run the relay:

```bash
RELAY_ENDPOINT_ID=dev-worker-01 \
NATS_URL=nats://localhost:4222 \
HMAC_SECRET=dev-secret-change-me \
ALLOW_PRIVATE_IPS=true \
go run ./cmd/relay
```

Proxy a request:

```bash
curl -X POST http://localhost:8080/v1/request \
  -H "Content-Type: application/json" \
  -d '{"method":"GET","url":"https://httpbin.org/get"}'
```

## Configuration

Relay:

- `RELAY_ENDPOINT_ID`: endpoint worker ID to target. Falls back to `ENDPOINT_ID`.
- `NATS_URL`, `NATS_TOKEN`: NATS connection.
- `HMAC_SECRET`: shared signing secret. Must match the endpoint.
- `HTTP_PORT`: relay listen port. Default `8080`.
- `RESULT_TIMEOUT`: endpoint response timeout. Default `30s`.
- `MAX_BODY_SIZE`: relay request body limit. Default `2M`.
- `MAX_CONCURRENT_REQUESTS`: relay concurrency cap. Default `50`.
- `ALLOW_PRIVATE_IPS`: allow private target URLs. Default `false`.

Endpoint:

- `ENDPOINT_ID`: worker ID. The relay targets this value.
- `NATS_URL`, `NATS_TOKEN`: NATS connection.
- `HMAC_SECRET`: shared signing secret. Must match the relay.
- `CONCURRENCY_LIMIT`: worker task concurrency. Default `25`.
- `MAX_POOL_HOSTS`, `IDLE_CONNS_PER_HOST`, `IDLE_CONN_TIMEOUT`: endpoint HTTP transport pool tuning.

## Development

```bash
make build
make test
make lint
```

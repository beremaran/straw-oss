# CLI

Build the CLI from the repository root:

```sh
go build -o ./bin/straw ./cmd/straw
./bin/straw request --url https://example.com
```

Options:

```text
--base-url URL             Control URL (default STRAW_BASE_URL or http://localhost:8080)
--token TOKEN              deployment token (default STRAW_AUTH_TOKEN)
--method METHOD            upstream method (default GET)
--url URL                  absolute upstream URL (required)
--header 'Name: value'     repeatable upstream header
--body-file PATH           send a file as the request body
--receipt-id ID            send a verified request receipt (exclusive with --body-file)
--response-body-mode MODE  inline_base64 or receipt
--timeout-ms N             total request deadline
--fingerprint-profile NAME outbound fingerprint profile
--route-tag TAG            required worker tag; repeatable
--route-country CODE       required worker country
--route-region NAME        required worker region
--route-ip-type NAME       required worker IP type
--sticky-session-id ID     sticky worker session identifier
--replayable               permit transport retry when safe to replay
```

The CLI writes the same JSON response returned by the API. For authenticated deployments:

```sh
export STRAW_BASE_URL=https://straw.example.net
export STRAW_AUTH_TOKEN='replace-me'
straw request --url https://example.com
```

`--base-url` and `--token` override `STRAW_BASE_URL` and `STRAW_AUTH_TOKEN`; explicit flags always win. The CLI does
not read a configuration file. `--body-file` reads the file once and is exclusive with `--receipt-id`; size remains
subject to Control limits. Repeated `--header` and `--route-tag` values preserve their order. Routing flags are sent
as the same `routing` object used by REST and the Go/Python SDKs. `--replayable` explicitly opts a non-default method
into transport retry; GET, HEAD, and OPTIONS remain replayable by the tagged Go SDK default.

Exit status `0` means the command completed and JSON was written to stdout. Status `1` covers usage, file, network,
deadline, API, and output failures; diagnostics go to stderr and successful JSON never does. Fields in the JSON
response follow the REST compatibility policy. Scripts should inspect the response/error `code`, not prose or field
ordering. `help`, `-h`, and `--help` print usage to stdout. Unknown commands and invalid flags fail without a request.

The process propagates its context deadline and operating-system interruption to the HTTP request. It does not retry
automatically. Use `--receipt-id` only after completing a request receipt; use `--response-body-mode receipt` and the
Receipt API to download stored responses. Shell completion is not shipped.

## Process healthcheck flags

The `control` and `egress` binaries accept `-config <path>` and `-healthcheck`. Healthcheck mode probes the local
readiness endpoint and exits zero only when ready; errors go to stderr and produce a non-zero exit status.

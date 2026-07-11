---
sidebar_position: 6
---

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
--timeout-ms N             total request deadline
--fingerprint-profile NAME outbound fingerprint profile
```

The CLI writes the same JSON response returned by the API. For authenticated deployments:

```sh
export STRAW_BASE_URL=https://straw.example.net
export STRAW_AUTH_TOKEN='replace-me'
straw request --url https://example.com
```

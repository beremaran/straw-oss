# Handoff

Task: `docs/tasks/p0/11-egress-outbound-execution.md`

## Changed

- `internal/egress/executor.go`: added the P0 outbound HTTP/HTTPS executor, header/cookie injection, total-deadline enforcement, resolved-IP destination policy enforcement, DNS rebinding guard, and canonical executor error frames.
- `internal/egress/executor_test.go`: added focused tests for successful execution, header/cookie injection, total deadline, resolved-IP deny, private/metadata denial, DNS rebinding, redirect passthrough, P0 transport defaults, unsafe injection, and redaction boundaries.
- `cmd/egress/main.go`: constructs the P0 executor defaults after config and NATS server validation.
- `docs/tasks/p0.md` and `docs/tasks/p0/11-egress-outbound-execution.md`: marked task 11 complete after verification.

## Verification

```sh
go test ./internal/egress
go test ./...
make check
```

Result:

- Focused Egress tests pass.
- `go test ./...` passes.
- `make check` passes: `go test ./...` passes and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports `0 issues`.

## Reviewer Start Points

- `internal/egress/executor.go`
- `internal/egress/executor_test.go`
- `cmd/egress/main.go`

## Transport Defaults

- Redirects are not followed; redirect responses pass through as upstream responses.
- Outbound HTTP/2 is disabled with `ForceAttemptHTTP2=false` and an empty `TLSNextProto` map.
- Upstream keep-alives are disabled with `DisableKeepAlives=true`.
- Direct-local resolution validates every resolved IP and dials only the selected validated IP.

## Emitted Error Facts

- `invalid_request_start` -> `ERROR_CODE_EXECUTOR_INTERNAL_ERROR`
- `header_injection_failed` -> `ERROR_CODE_EXECUTOR_INTERNAL_ERROR`
- `invalid_destination_policy` -> `ERROR_CODE_EXECUTOR_INTERNAL_ERROR`
- `unsupported_resolution_mode` -> `ERROR_CODE_DESTINATION_DENIED`
- `unsupported_fingerprint_profile` -> `ERROR_CODE_UNSUPPORTED_FINGERPRINT`
- `dns_denied_ip` -> `ERROR_CODE_DESTINATION_DENIED`
- `dns_no_records` -> `ERROR_CODE_UPSTREAM_DNS_FAILURE`
- `tcp_refused` -> `ERROR_CODE_UPSTREAM_CONNECTION_REFUSED`
- `tls_handshake_failed` -> `ERROR_CODE_UPSTREAM_TLS_FAILURE`
- `deadline_exceeded_total` -> `ERROR_CODE_TIMEOUT_EXCEEDED` with `TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT`
- `upstream_reset_before_headers` -> `ERROR_CODE_UPSTREAM_RESET`
- `executor_internal_error` -> `ERROR_CODE_EXECUTOR_INTERNAL_ERROR`

## Remaining Work

- (Corrected by audit 2026-07-03.) The executor is never invoked by the running binary:
  `cmd/egress/main.go` constructs it and discards it (`_ = egress.NewExecutor(...)`), and no
  assignment-consumption loop exists. Live invocation is owned by
  `docs/tasks/p0/23-egress-assignment-execution-loop.md`.
  [Update 2026-07-07 sweep: resolved by `docs/tasks/p0/23-egress-assignment-execution-loop.md`; `cmd/egress/main.go`
  now constructs the executor and passes it into `egress.Run`, which starts the assignment loop.]

## Blockers

- None.

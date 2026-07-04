# Handoff

Task: `docs/tasks/p0/33-ingress-header-crlf-validation.md`

## Changed

- `internal/control/request.go`: `validateHeaders` now checks the base64-decoded header value for a bare CR
  (`\r`, 0x0D) or LF (`\n`, 0x0A) and rejects with `invalid_request` if either is present. Per-header checks
  (name length, HTTP token validity, `Host` rejection, raw-string CR/LF, base64 decode, decoded CR/LF) were
  extracted into `validateHeaderPair` to keep `validateHeaders` under the `cyclop` complexity limit after adding
  the new check. The existing raw base64-string CR/LF check and the aggregate-byte-count enforcement are
  unchanged.
- `internal/control/handler_test.go`: `TestValidateRequestCRInHeaderValue` now sends `value_base64: "YQ0KYg=="`
  (base64 for `a\r\nb`) — valid base64 whose decoded bytes contain CR/LF — and asserts a `ValidationError` with
  `errorCodeInvalidRequest`, replacing the placeholder assertion that documented the gap.
- `docs/tasks/p0/33-ingress-header-crlf-validation.md`, `docs/tasks/p0.md`: marked done.
- `docs/agents/testing-matrix-audit.md`: no edit needed. The "HTTP validation" row already lists
  `TestValidateRequestCRInHeaderValue` as the backing test and never referenced the egress fallback, so it was
  already correct once the test itself was fixed.

Only bare CR (`\r`) and LF (`\n`) are rejected, matching the task scope and the egress-side `safeOutboundHeader`
check in `internal/egress/executor.go` (`bytes.ContainsAny(value, "\r\n")`) — no other control characters were
added to either side, keeping the two checks symmetric.

## Verification

```sh
go test ./internal/control/... -run TestValidateRequestCRInHeaderValue -v
make check
```

Result: both pass. `make check` initially flagged `cyclop: calculated cyclomatic complexity for function
validateHeaders is 11, max is 10` after adding the new branch; fixed by extracting `validateHeaderPair` rather
than suppressing the linter.

## Reviewer Start Points

- `internal/control/request.go:215` (`validateHeaders`, `validateHeaderPair`)
- `internal/control/handler_test.go:672` (`TestValidateRequestCRInHeaderValue`)

## Remaining Work

- None. This task only closed the ingress-side gap; the egress-side `safeOutboundHeader` defense-in-depth check
  is untouched, as required by the task's Out of Scope section.

## Blockers

- None.

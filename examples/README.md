# Runnable examples

Tested with Straw `main`, Go 1.26.5, Python 3.13, and SDK tags in `docs/public/compatibility.md`. Start `make dev`
first. The default endpoint is `http://localhost:8080`; set `STRAW_BASE_URL` and `STRAW_AUTH_TOKEN` for another
deployment. Successful examples print upstream status `200`. Stop with `make dev-down`.

- `curl/request.sh`: REST request with ordered duplicate headers and inline body.
- `cli/request.sh`: build and invoke the CLI with a bounded timeout.
- `go/main.go`: Go client lifecycle with context timeout and typed API error handling.
- `python/request.py`: Python client request and API error handling.

Never retry a non-idempotent upstream request unless application semantics make replay safe. Receipt workflows are
maintained in `docs/public/object-storage-receipts.md` because they require the optional profile.

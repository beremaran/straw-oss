# Testing Guide

The default verification command is:

```sh
make check
```

For focused work, run the smallest useful command first, then `make check` before handoff.

Examples:

```sh
go test ./internal/config
go test ./internal/control/...
go test ./...
```

P0 requires table-driven tests for contracts, routing, worker state, NATS subjects, request lifecycle, error mapping, Redis failure policy, deny rules, and ClickHouse write behavior. Use `docs/planning/30-testing-matrix.md` for required coverage.

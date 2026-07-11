# Executable fingerprint profiles

**State:** Delivered

Straw accepts named browser-transport profiles as a bounded proxy capability. Control admits a requested profile only
when the selected Egress worker advertises it, and the worker independently rejects any assignment it cannot execute.
The baseline transport remains available when no named profile is requested.

## Supported catalog

- `baseline`: standard Go HTTP transport.
- `default`: compatibility alias resolved to `baseline` before dispatch.
- `chrome_120`: executable browser-like TLS and HTTP transport backed by the pinned `tls-client` profile.

The executable catalog is finite and code-owned. Database rows may enable or disable routing availability, but they
cannot make an unknown profile executable. Tenant input cannot define arbitrary TLS settings.

## Trust and execution boundary

Control:

- validates the requested name and tenant-visible catalog state;
- selects only a worker whose signed capabilities contain the profile;
- preserves destination policy, deadline, cancellation, and request evidence;
- records requested, selected, and executed profile values in ClickHouse.

Egress:

- resolves the instruction against its compiled executable registry;
- refuses capability drift before making an upstream connection;
- preserves validated-IP dialing, original-host SNI, certificate hostname validation, redirect policy, and response
  streaming semantics;
- maps unsupported or failed profile execution to the canonical public error registry.

The implementation lives in `internal/control/`, `internal/egress/profile_registry.go`,
`internal/egress/profiled_transport.go`, the signed Protobuf registration contract, and migration
`migrations/postgres/0012_executable_fingerprint_profiles.sql`.

## Operational evidence

`deploy/local/clickhouse-schema.sql` owns the request-event evidence columns:

- `requested_fingerprint_profile`
- `selected_fingerprint_profile`
- `executed_fingerprint_profile`

The local migration and its clean/existing-volume check live under `deploy/local/scripts/`. Dashboards and Prometheus
configuration live under `deploy/local/observability/`.

## Verification

```sh
make check
make clickhouse-migrations-check
docker compose -f deploy/local/docker-compose.yml config -q
```

Security and architecture-governance conclusions are retained in
[`../security/executable-fingerprint-profiles.md`](../security/executable-fingerprint-profiles.md). Canonical protocol,
execution, observability, and security decisions remain under `docs/planning/`.

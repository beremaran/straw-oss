## 6. Identity, Roles, and Tenant Isolation

### Tenant Resolution

Control derives `tenant_id` exclusively from the validated API key or worker credential. Clients and workers cannot
provide or override tenant identity through headers, request metadata, request body fields, or NATS subject tokens.

### Role Scopes

Straw has two role scopes:

- platform scope, for operations that create or delete tenant boundaries;
- tenant scope, for operations inside a tenant boundary.

P0 must not rely on a tenant-scoped role to create that same tenant. Local development may bootstrap a default platform
administrator and default tenant through seed data or migration fixtures.

### Platform Role

| Role           | Scope    | Purpose                                                              |
|----------------|----------|----------------------------------------------------------------------|
| `system_admin` | Platform | Create, soft-delete, and administer tenants; bootstrap tenant admins |

### Tenant Roles

Tenant roles apply only inside one tenant.

| Role           |      Data-plane execution |                      Config mutation | Credential management |            Telemetry read | Payload capture control |
|----------------|--------------------------:|-------------------------------------:|----------------------:|--------------------------:|------------------------:|
| `requester`    |                       Yes |                                   No |                    No | Own request metadata only |                      No |
| `viewer`       |                        No |                                   No |                    No |                       Yes |                      No |
| `operator`     | Optional by tenant policy | Routing/fingerprint/injection config |                    No |                       Yes |                      No |
| `tenant_admin` |                       Yes |                                  Yes |                   Yes |                       Yes |                     Yes |

Where earlier text says `admin` for tenant-local actions, it means `tenant_admin`. Where tenant creation or deletion is
required, the role is `system_admin`.

API keys inherit the role and tenant of the user/key record. P0 supports tenant-scoped API keys. Cross-tenant users use
separate keys per tenant. Platform administration uses a separate platform-scoped credential and is not accepted for
data-plane request execution unless it also maps to a tenant role.

### Worker Credential Scope

Worker credentials bind to:

- credential ID,
- tenant scope or explicit multi-tenant scope,
- allowed pool IDs,
- executor type,
- signing public key,
- status.

A worker cannot register pools, countries, regions, tags, ingress modes, IP types, or other capabilities outside its
credential scope.

### Tenant Isolation Rules

Tenant isolation applies to:

- route snapshots,
- routing rules,
- API keys,
- worker credentials,
- worker pool membership,
- sticky sessions,
- rate-limit counters,
- quota counters,
- ClickHouse records,
- object-storage BodyRefs,
- request IDs and trace correlation.

A request for tenant A must never route to an executor credentialed only for tenant B. A multi-tenant worker may execute
for multiple tenants only when its credential explicitly grants that scope and the selected pool is eligible for the
requesting tenant.

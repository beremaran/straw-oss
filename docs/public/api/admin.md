# Runtime Administration APIs

Straw provides runtime control APIs to inspect active Egress workers, enable/disable/drain workers, and cancel in-flight requests.

---

## 1. Worker Management

### List Workers
- **Endpoint**: `GET /api/v1/admin/workers`
- **Authentication**: Required (`Authorization: Bearer <api_key>`)
- **Required Role**: `system_admin`, `tenant_admin`, or `operator`
- **Behavior**:
  - **Platform Scoped (`system_admin`)**: Returns all registered workers with complete runtime data (including NATS subjects and session identifiers).
  - **Tenant Scoped (`tenant_admin` / `operator`)**: Returns only Egress workers eligible to route requests for the caller's tenant. Sensitive details like NATS `assign_subject` and `session_id` are redacted.

#### Example Tenant Scope Response:
```json
[
  {
    "worker_id": "worker_us_west_1",
    "runtime_state": "connected",
    "global_admin_state": "enabled",
    "global_draining": false,
    "tenant_admin_state": {
      "22222222-2222-4222-8222-222222222222": "enabled"
    },
    "tenant_draining": {
      "22222222-2222-4222-8222-222222222222": false
    },
    "executor_type": "egress"
  }
]
```

---

### Global Worker State Actions
These actions require a platform-scoped `system_admin` API key and affect worker availability across all tenants.

- **Globally Disable Worker**: `POST /api/v1/admin/workers/{worker_id}/disable`
  - *Prevents the worker from receiving any new request assignments across all tenants.* Persisted in Postgres.
- **Globally Enable Worker**: `POST /api/v1/admin/workers/{worker_id}/enable`
  - *Restores the worker to active availability status.* Persisted in Postgres.
- **Globally Drain Worker**: `POST /api/v1/admin/workers/{worker_id}/drain`
  - *Drains the worker's active request queue. The worker will not receive new assignments, but is allowed to complete already dispatched requests.* (Runtime-only; not persisted).
- **Globally Stop Drain**: `POST /api/v1/admin/workers/{worker_id}/undrain`
  - *Stops draining the worker and resumes active assignments.* (Runtime-only).

---

### Tenant-Scoped Worker State Overrides
These actions affect worker eligibility only for the caller's tenant.

- **Disable Worker for Tenant**: `POST /api/v1/admin/workers/{worker_id}/tenant-disable`
  - **Required Role**: `tenant_admin`
  - *Prevents the worker from receiving assignments for the caller's tenant.* Persisted in Postgres.
- **Enable Worker for Tenant**: `POST /api/v1/admin/workers/{worker_id}/tenant-enable`
  - **Required Role**: `tenant_admin`
  - *Restores eligibility of the worker for the caller's tenant.* Persisted in Postgres.
- **Drain Worker for Tenant**: `POST /api/v1/admin/workers/{worker_id}/tenant-drain`
  - **Required Role**: `tenant_admin` or `operator`
  - *Stops sending new assignments from the tenant to this worker while allowing active in-flight tenant requests to finish.* (Runtime-only).
- **Stop Tenant Drain**: `POST /api/v1/admin/workers/{worker_id}/tenant-undrain`
  - **Required Role**: `tenant_admin` or `operator`
  - *Resumes normal assignments from the tenant to this worker.* (Runtime-only).

> [!IMPORTANT]
> Global admin decisions take precedence: a globally disabled worker will never receive requests, even if a tenant has marked that worker as enabled.

---

## 2. Request Cancellation

Allows administrators and operators to cancel long-running or stalled requests that are currently in-flight.

- **Endpoint**: `POST /api/v1/admin/requests/{request_id}/cancel`
- **Required Role**: `system_admin`, `tenant_admin`, or `operator`
- **Behavior**:
  - **Platform Scoped (`system_admin`)**: Can cancel any active in-flight request in the system.
  - **Tenant Scoped (`tenant_admin` / `operator`)**: Can only cancel an in-flight request that belongs to their own tenant.
  - **Response Status**: `200 OK` on success, indicating that the cancellation signal has been sent.

#### Example Success Response:
```json
{
  "request_id": "req_1783260685717525503",
  "status": "cancelling"
}
```

> [!WARNING]
> If a tenant-scoped API key attempts to cancel a request ID that does not exist or belongs to another tenant, the API returns a `403 Forbidden` status with the code `insufficient_permissions`. This prevents tenants from probing the existence of other tenants' request traces.

# Security Model

Security is a primary design concern for the Straw Proxy Server. The system employs multiple layers of protection for both client access and internal communication.

## Client Authentication

Clients authenticate using Bearer tokens.

* **Method**: `Authorization: Bearer <token>` header.
* **Storage**: Tokens are securely hashed using **SHA256** before storage in PostgreSQL.
* **Scopes**: Keys can be restricted to specific tags (e.g., a key can be limited to `target:amazon` only).

## Internal Security

### Relay ↔ Endpoint Authentication

Communication between the Server and Endpoints is secured to prevent unauthorized task execution.

1. **Broker Auth**: RabbitMQ connections use TLS and standard authentication (username/password or mTLS).
2. **Payload Signing**: Every task directed to an endpoint includes an **HMAC-SHA256 signature**. Endpoints verify this signature using a shared secret before executing any request.

```go
type SignedTask struct {
    Payload   []byte `json:"payload"`   // Compressed request
    Signature string `json:"signature"` // HMAC-SHA256
    Timestamp int64  `json:"ts"`        // Replay protection window
}
```

### Upstream TLS Verification

* **Default**: Endpoints strictly verify target website certificates (`InsecureSkipVerify: false`).
* **Override**: Specific routing rules can allow insecure TLS (`AllowsInsecureTLS: true`) for testing or internal targets, but this is logged heavily.

## Secrets Management

* Secrets (DB passwords, HMAC keys) are loaded via environment variables or HashiCorp Vault.
* **Zero-Trust**: Secrets are never logged or returned in error responses.

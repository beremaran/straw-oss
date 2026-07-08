# Proxy & Ingress Modes

Straw provides four distinct ingress modes for routing outbound HTTP and HTTPS traffic through the distributed egress plane. Whether your client applications require granular programmatic control via REST JSON envelopes, seamless drop-in proxying for existing HTTP clients, TCP tunneling, or full TLS terminating inspection, Straw supports your architecture.

---

## Supported Ingress Modes Overview

| Ingress Mode | Port (Default) | Protocol | TLS Termination | Body Inspection / Mutation | Header Injection | Common Use Cases |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **REST & Streaming** (`rest`) | `8080` | HTTP/HTTPS (POST) | Controlled via API | Yes (inline or S3 reference) | Yes (via policy & API) | Programmatic integration, SDKs, custom error wrappers, structured JSON pipelines. |
| **HTTP Forward Proxy** (`http_proxy`) | `8081` | Plaintext HTTP | N/A (Plaintext HTTP) | Yes | Yes | Drop-in forward proxy for standard scrapers, web crawlers, and HTTP libraries. |
| **HTTP CONNECT Tunnel** (`connect`) | `8082` | HTTP CONNECT Tunnel | No (End-to-end TLS) | No (Encrypted TCP stream) | No | Private HTTPS tunneling where end-to-end TLS privacy between client and upstream is required. |
| **MITM TLS Inspection** (`mitm`) | `8083` | HTTPS with Custom CA | Yes (Re-encrypted by Egress) | Yes | Yes | Deep payload inspection, data loss prevention (DLP), audit logging, and header injection for HTTPS. |

---

## Mode 1: REST & Streaming API (`rest`)

The REST ingress mode is Straw's default programmatic interface. Clients submit JSON payloads to Control plane endpoints (`POST /api/v1/requests` or `POST /api/v1/requests:stream`), specifying the target destination, method, custom headers, timeout, and optional body.

### Why Use REST Ingress?
- **Granular Control**: Specify exact routing parameters, timeout budgets, and custom header overrides per request.
- **Structured Error Handling**: Upstream HTTP status codes and errors are cleanly wrapped in JSON response envelopes without obscuring network-level failures.
- **Large Payload Support**: Supports inline base64 data as well as out-of-band payload references via S3 storage buckets.
- **Streaming**: Real-time server-sent event (SSE) or chunked streaming via `POST /api/v1/requests:stream`.

### Authentication
Include the API key in the standard HTTP `Authorization` header:
```http
Authorization: Bearer sk_example_req_...
```

For complete schemas, request normalizations, and error code breakdowns, consult the [REST Request Forwarding Reference](api/requests.md).

---

## Mode 2: Standard HTTP Forward Proxy (`http_proxy`)

When `server.proxy_enabled` is set to `true` on the Control plane, Straw exposes a standard HTTP forward proxy listener on port `8081`. Standard HTTP libraries, scripts, and command-line tools can route traffic through Straw simply by configuring their HTTP proxy setting.

### How It Works
1. The client sends a standard HTTP request to the proxy listener (`http://control:8081`).
2. Control authenticates the request using the proxy authorization header.
3. The request is evaluated against tenant quotas, rate limits, and routing rules.
4. Control schedules the work over NATS to an eligible Egress worker capable of `http_proxy` execution.
5. The worker executes the HTTP request against the public internet and returns the raw HTTP response headers and body directly to the client.

### Authentication
Standard forward proxies use the `Proxy-Authorization` header. Straw requires the Bearer scheme:
```http
Proxy-Authorization: Bearer sk_example_req_...
```

### Example Usage
```bash
# Using curl with the standard proxy flag (-x) and Proxy-Authorization header
curl -x http://localhost:8081 \
  -H "Proxy-Authorization: Bearer sk_example_requester_secret" \
  http://example.com/
```

> [!NOTE]
> The `http_proxy` mode only supports plaintext HTTP targets. For HTTPS destinations without TLS termination, use the `connect` mode.

---

## Mode 3: HTTP CONNECT Tunneling (`connect`)

When `server.connect_enabled` is set to `true`, Straw opens a TCP tunneling proxy listener on port `8082`. This mode implements the standard HTTP `CONNECT` method, allowing clients to establish end-to-end encrypted TLS tunnels through Straw to upstream destinations.

### How It Works
1. The client initiates an HTTP `CONNECT host:443 HTTP/1.1` request to the Control plane listener on port `8082`.
2. Control validates the API key in the `Proxy-Authorization` header and evaluates destination deny rules.
3. Control assigns the connection to an Egress worker supporting `connect` ingress mode.
4. A bidirectional TCP relay is opened over NATS between the client and the Egress worker.
5. The client performs its TLS handshake directly with the upstream destination server. Straw sees only encrypted ciphertext and cannot modify headers or inspect request/response bodies.

### Authentication
Include your Bearer token in the proxy authorization header during the initial `CONNECT` handshake:
```http
Proxy-Authorization: Bearer sk_example_req_...
```

### Example Usage
```bash
# Use curl -p (proxytunnel) to establish a CONNECT tunnel through Straw port 8082
curl -p -x http://localhost:8082 \
  -H "Proxy-Authorization: Bearer sk_example_requester_secret" \
  https://example.com/
```

> [!TIP]
> **When to choose CONNECT mode**: Choose `connect` when your compliance or security policy mandates strict end-to-end encryption between client workloads and upstream servers, precluding middlebox TLS decryption.

---

## Mode 4: MITM TLS Inspection Proxy (`mitm`)

When `server.mitm_enabled` is set to `true`, Straw operates a full TLS-terminating inspection proxy on port `8083`. This mode enables deep packet inspection, header injection, fingerprint profile application, and payload capture for HTTPS traffic.

### How It Works
1. When an HTTPS client connects to port `8083`, Straw intercepts the TLS handshake and dynamically generates an ephemeral TLS certificate signed by Straw's internal Root Certificate Authority (CA).
2. The Egress worker decrypts the request payload, applies any tenant-configured injection policies (e.g., adding API tokens or custom User-Agents), and applies browser TLS fingerprinting profiles (`chrome_120`, `firefox_120`, `safari_16_0`).
3. The Egress worker initiates an independent, clean TLS connection to the real upstream server, receives the response, records telemetry/payload capture data if configured, and re-encrypts the response back to the client.

### Authentication
For MITM inspection requests on port `8083`, pass your API key via standard proxy authorization:
```http
Proxy-Authorization: Bearer sk_example_req_...
```

### Root CA Management & Trust
To prevent TLS certificate verification errors in client applications, clients must download and trust Straw's active MITM Root CA certificate.

#### Downloading the Public Root CA
Any authenticated tenant or operator with data-plane access can download the public certificate in PEM format from the Control plane:

```bash
curl -s -H "Authorization: Bearer sk_example_requester_secret" \
  http://localhost:8080/api/v1/mitm/ca.pem -o straw_ca.pem
```

#### Using the CA with HTTP Clients
Once downloaded, instruct your client tools or system trust store to use `straw_ca.pem`:

```bash
# Pass --cacert to curl when routing HTTPS traffic through the MITM proxy on port 8083
curl --cacert straw_ca.pem \
  -x http://localhost:8083 \
  -H "Proxy-Authorization: Bearer sk_example_requester_secret" \
  https://example.com/
```

#### Rotating Root CA Certificates
Tenant administrators or system administrators can dynamically rotate the MITM CA certificate and private key without restarting the cluster using the REST API:

```bash
curl -X PUT http://localhost:8080/api/v1/mitm/ca \
  -H "Authorization: Bearer sk_example_tenant_admin_secret" \
  -H "Content-Type: application/json" \
  -d '{
    "cert_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
    "key_pem": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
  }'
```

> [!IMPORTANT]
> **Tenant Rule Enforcement**: For a tenant to utilize MITM inspection or download `ca.pem`, the tenant must have at least one active routing rule whose `ingress_type` is either empty (`""` matching all modes) or explicitly set to `"mitm"`.

---

## Worker Capabilities & Routing Match Conditions

When configuring your deployment, ensure that both your Egress worker credentials and your tenant routing rules align with your chosen ingress modes.

### 1. Worker Capabilities
Each registered Egress worker advertises its supported proxy modes in its registration capabilities (`supported_ingress_modes`). By default, workers support `["rest"]`. To enable forward proxying, tunneling, or inspection, include the relevant modes when provisioning worker credentials:

```json
{
  "executor_type": "egress",
  "public_key_ed25519_base64": "...",
  "capabilities": {
    "supported_ingress_modes": ["rest", "http_proxy", "connect", "mitm"],
    "supported_fingerprints": ["default", "chrome_120", "firefox_120", "safari_16_0"]
  }
}
```

### 2. Routing Rule Match Conditions
Tenant routing rules can be scoped to apply only when requests arrive via a specific ingress mode by setting `match.ingress_type`:

```json
{
  "name": "Route MITM traffic to dedicated pool",
  "enabled": true,
  "priority": 10,
  "match": {
    "target_host": "*.api.partner.com",
    "ingress_type": "mitm"
  },
  "target_pool_id": "pool_secure_inspection"
}
```

If `ingress_type` is omitted or left empty (`""`), the routing rule matches traffic arriving across all supported ingress modes.

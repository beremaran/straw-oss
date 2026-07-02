The Straw Proxy Plan outlines a robust, horizontally scalable architecture, but it contains specific contradictions,
coverage blind spots, and incomplete mechanical details that will cause issues during implementation.

### Consistency

* **Surveillance Contradiction:** The plan explicitly states MITM interception is not for surveillance or general
  man-in-the-middle use. It subsequently details a Payload Capture feature that stores full, truncated, or metadata-only
  request and response bodies in ClickHouse.


* **Zero-Interpretation vs. Redaction:** The security section claims Straw acts as a "Zero-Interpretation Transport"
  that treats bodies as unparsed data. The configuration section contradicts this by introducing regex-based redaction
  rules that match and modify body content patterns before storage.


* **NATS Payload Terminology:** The document uses the term "payload" ambiguously. It refers to internal protobuf binary
  messages as NATS payloads. It also refers to captured HTTP bodies as payloads.

---

### Coverage

* **Architectural Boundaries:** The separation of concerns between Control (routing, auth, state) and Egress (stateless
  execution, outbound TLS) is comprehensively defined.


* **State Isolation:** The distinct use cases for Postgres (durable config), Redis (ephemeral state), and ClickHouse (
  analytics) provide excellent coverage for system scaling.


* **Provider Adapters:** Provider Adapters are scoped as Phase 1 components to bypass Egress workers for vendor
  endpoints. The plan fails to cover how these adapters handle vendor-specific rate limits, API pagination, or custom
  error translation beyond basic round-robin load balancing.


* **Control-DNS Missing:** The Egress execution section mentions workers can opt into a caching DNS resolver maintained
  by Control. This Control-maintained DNS component is entirely absent from the System Overview, Components, and
  Configuration sections.

---

### Completeness

* **Regex on Compressed Streams:** Egress workers preserve upstream `Content-Encoding` and do not decompress responses.
  Payload capture tees this stream and applies body pattern redaction rules. The plan is incomplete because it does not
  explain how Control will execute regex redaction on raw, compressed byte streams.


* **Long-Lived HTTP Streams:** The plan enforces a hard, unified request deadline that wraps the entire outbound
  lifecycle. It does not provide a mechanism to handle legitimate, long-lived HTTP streams (such as Server-Sent Events
  or large, slow downloads) without them failing against this absolute timeout cap.


* **Large-Body Garbage Collection:** Bodies exceeding configured message limits are routed to S3-compatible object
  storage with a strict 3-day retention lifecycle. The plan lacks a garbage collection strategy for orphaned S3 chunks
  resulting from aborted client uploads or worker crashes mid-stream.


* **CGO Bottlenecks:** The plan acknowledges that `tls-client` relies on CGO, presenting a severe blocking risk to
  high-concurrency requests in Egress workers. It fails to address that the Control node also relies on `tls-client` for
  inbound TLS termination.


* **Cache-Miss Thundering Herds:** Control mitigates dynamic MITM certificate generation storms using a local
  `singleflight` group and Redis locks. The plan does not detail how Control CPU will survive the CGO initialization
  overhead when a massive volume of concurrent requests for *unique* SNIs bypasses the `singleflight` deduplication.
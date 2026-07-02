## 19. Payload Capture — P2

Payload capture is off by default and explicitly enabled by tenant admin policy.

### Capture Boundary

Payload capture is a tee. It must not mutate forwarded request or response bytes.

Redaction applies only to stored copies.

Live payload/header redaction is not a Phase 1/P2 feature unless separately designed as traffic mutation. The only live
outbound mutation in this plan is explicit header/cookie injection.

### Capture Decisions

Capture decision enum:

- `NONE`,
- `METADATA_ONLY`,
- `HEADERS`,
- `BODY_TRUNCATED`,
- `BODY_FULL`.

Even `BODY_FULL` is bounded by configured capture limits. Unlimited full-body capture into ClickHouse is not supported.

### Compression and Parsing

If the body is compressed and Straw does not decompress it, body regex/JSONPath redaction cannot inspect the plaintext.
In that case:

- store raw compressed bytes only if allowed, or
- store metadata only, or
- defer body redaction/decompression to a future feature.

P2 supports header redaction and raw-body truncation. Body regex/JSONPath redaction requires an explicit decoding
pipeline and is not part of baseline P2.

### Storage

Captured payload records store metadata in ClickHouse and large captured bodies in object storage by reference when they
exceed ClickHouse-safe limits.

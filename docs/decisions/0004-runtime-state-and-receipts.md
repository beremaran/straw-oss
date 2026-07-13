# ADR 0004: snapshot ownership, Redis fencing, and receipt verification

Status: accepted. Date: 2026-07-13. Owner: runtime maintainer.

Control validates and atomically activates versioned runtime snapshots; workers acknowledge applied versions.
JetStream owns durable configuration. Redis owns only expiring HA coordination and uses fenced ownership. Receipt
metadata and objects are durable optional state; declared size and SHA-256 are verified before readiness and again at
assignment consumption. Assignment URLs bind deployment, request, receipt, and expiry.

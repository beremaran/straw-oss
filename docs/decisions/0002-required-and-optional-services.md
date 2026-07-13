# ADR 0002: NATS is the only required service

Status: accepted. Date: 2026-07-13. Owner: maintainer.

Core NATS carries bounded assignment and stream messages. JetStream is optional durable runtime configuration, Redis
is optional HA coordination with fencing/TTLs, and object storage is optional receipt transport. The default profile
must remain operable with only NATS. Optional service failure must have explicit readiness and degraded behavior.

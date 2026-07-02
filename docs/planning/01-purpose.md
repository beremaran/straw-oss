## 1. Purpose

Straw is a distributed HTTP/HTTPS proxy system for high-scale scraping and controlled outbound HTTP transport. It
centralizes ingress, authentication, authorization, tenant isolation, routing policy, and observability in a Control
service, while outbound execution happens through registered executors.

Straw lets operators combine:

- operator-owned Egress Workers,
- operator-configured upstream proxies,
- optional Provider Adapters for direct provider/vendor execution,
- policy-based route selection,
- stable transport error semantics,
- browser-like outbound TLS behavior where supported by the executor.

Straw is not an anonymity network, scraping orchestrator, browser automation system, CAPTCHA solver, compliance engine,
or unmanaged public proxy. It transports requests through configured routes and exposes predictable operational control
over that transport.

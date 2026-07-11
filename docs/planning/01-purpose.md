## 1. Purpose

Straw is a self-hosted, distributed HTTP/HTTPS egress proxy. It gives one operator-controlled deployment a public
Control endpoint and one or more Egress workers without turning the project into a hosted platform.

Straw lets operators combine:

- operator-owned Egress workers,
- operator-configured upstream proxies,
- custom Egress implementations built with the Egress SDK,
- deployment-wide route and destination policies,
- stable transport error semantics,
- browser-like outbound TLS behavior where supported by the executor.

The open-source distribution deliberately excludes multi-tenancy, tenant lifecycle APIs, platform RBAC, quotas or
billing, database-backed configuration management, and multi-Control coordination. Those concerns obscure the core
proxy and require infrastructure that is not useful to most self-hosters.

Straw is not an anonymity network, hosted proxy platform, scraping orchestrator, browser automation system, CAPTCHA
solver, compliance engine, or unmanaged public proxy. It transports requests through operator-configured routes and
exposes predictable operational control over that transport.

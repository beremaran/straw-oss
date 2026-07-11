---
name: update-straw-documentation
description: Write or update Straw's public documentation under docs/public for the self-hosted open-source proxy.
---

# Update Straw documentation

Use this skill for public guides, API references, quickstarts, deployment documentation, and other content published by
the Docusaurus site.

## Source of truth

Document shipped behavior. Check the relevant implementation, tests, `deploy/local`, `deploy/production`, and
`ROADMAP.md` before writing. One Straw deployment is one trust boundary and NATS is the only required backing
service. Do not document tenants, RBAC, quotas, billing, administration APIs, or mandatory databases as supported.

## Public information architecture

- `index.md`: product boundary and route into the manual
- `quickstart.md`: clone, `make dev`, first request, stack lifecycle
- `architecture.md`: Control, NATS, Egress, request lifecycle, trust boundary
- `configuration.md`: versioned Control/Egress JSON and environment secrets
- `api/requests.md`: the public REST request contract and error envelope
- `sdk.md`, `cli.md`, `egress_worker.md`: integration surfaces
- `deployment.md`, `operations.md`, `security.md`, `troubleshooting.md`: operating guidance
- `development.md`: contributor and release workflow

Update `website/sidebars.js` and `website/docusaurus.config.js` when pages or navigation change.

## Style

Write from installation through operation. Use task-oriented headings, runnable examples, explicit prerequisites,
defaults, limits, failure behavior, and security boundaries. Label production assets as adaptable examples, not a
turnkey platform. Never use placeholder behavior that the repository does not ship.

## Verification

Run examples against the local Compose stack when behavior changes. Build the site with:

```sh
make docs-website
```

Also run `make check` when documentation accompanies code or configuration changes.

## 31. Implementation order

The active outcome and remaining work live in the root `ROADMAP.md`.

1. Fix the open-source product boundary and remove multi-tenant/hosted-platform assumptions from canonical design.
2. Introduce deployment-scoped identity and static configuration behind focused behavior tests.
3. Switch request routing and worker registration to the simplified path.
4. Remove unreachable enterprise APIs, stores, migrations, configuration, dependencies, and tests.
5. Reduce local deployment to Straw and NATS, then verify a real request.
6. Convert production deployment assets into explicit patterns/templates.
7. Rewrite public documentation and community files from verified behavior.
8. Run full checks and complete a public-release hygiene review.

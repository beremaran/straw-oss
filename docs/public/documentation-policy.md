# Documentation policy

Last reviewed: 2026-07-13. Owner: documentation maintainer. Public pages document shipped behavior and use consistent
terms: Control, Egress worker, deployment, runtime snapshot, receipt, and trust boundary.

API/config/security pages require runtime or security-owner review; deployment/operations pages require an operator
review. Every major page carries owner and review date, runnable examples state prerequisites/expected output/cleanup,
and public-surface changes update the changelog and compatibility policy. Run `make docs-website` and applicable
examples. A quarterly sweep verifies owners, dates, links, commands, defaults, limits, failure behavior, and security
boundaries.

Use the [documentation issue template](https://github.com/beremaran/straw-oss/issues/new?template=documentation.yml)
or **Edit this page** link with page context. No analytics are collected today. The ownership manifest records at
least one tested command for every page; the ordinary documentation gate rejects missing ownership, review dates,
command evidence, or the contextual feedback route.
If privacy-compatible analytics are added, this page must first document purpose, retention, collection, and opt-out;
only failed searches, setup exits, and aggregated themes may guide new maintained recipes.

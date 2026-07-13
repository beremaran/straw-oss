# Documentation policy

Public pages document shipped behavior and use consistent terms: Control, Egress worker, deployment, runtime snapshot,
receipt, and trust boundary.

`owners.json` is the single record of page ownership. It holds the owner, the last review date, and at least one
tested command for every page under `docs/public`, so pages themselves carry no ownership or review metadata in their
prose. `make doc-ownership-check` rejects an unowned or unlisted page, missing command evidence, a review older than
the cycle recorded in the manifest, or a feedback route that is not the repository's contextual new-issue template.

API, configuration, and security pages require runtime or security-owner review; deployment and operations pages
require an operator review. Runnable examples state prerequisites, expected output, and cleanup, and public-surface
changes update the changelog and compatibility policy. `public-surface-check` verifies both source-declared names and
selected semantic anchors for high-risk API, security, deployment, operations, and integration contracts. Run
`make docs-check`, `make doc-ownership-check`, and
`make docs-website`, plus the applicable examples. A quarterly sweep re-verifies owners, dates, links, commands,
defaults, limits, failure behavior, and security boundaries.

Report a problem through the
[documentation issue template](https://github.com/beremaran/straw-oss/issues/new?template=documentation.yml) or a
page's **Edit this page** link, which carries the page context.

No analytics are collected today. If privacy-compatible analytics are added, this page must first document purpose,
retention, collection, and opt-out; only failed searches, setup exits, and aggregated themes may guide new maintained
recipes.

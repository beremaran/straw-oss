# Documentation and copy quality bar

Status: active standard. Owner: documentation maintainer.

Date: 2026-07-13

This document sets the quality bar for all public documentation and copy in `straw-oss`, records the 2026-07-13
copy audit against that bar, and complements `docs/public/documentation-policy.md` (process and ownership) by
defining the *writing* standard. The earlier structural audit lives in `documentation-audit-roadmap.md` and is
complete; this bar governs ongoing copy quality.

## The bar

The bar is derived from documentation practice in widely admired open-source projects:

- **FastAPI / Vite / Tailwind CSS** — a README that states what the project is in one sentence, shows a runnable
  quickstart above the fold, and links a full manual; status badges for CI, security, and license.
- **Kubernetes and Google developer style guides** — present tense, active voice, second person for instructions,
  one idea per sentence, no marketing adjectives, exact names for commands/fields/errors.
- **Stripe / SQLite documentation** — reference pages are contracts: every field, default, limit, error, and
  failure mode enumerated, with examples that are tested against the shipped implementation.
- **Rust / Go project norms** — honest scope statements ("what this does not do"), explicit pre-1.0 compatibility
  promises, and contributor docs that let a stranger make a change without maintainer memory.

Concretely, every public page must satisfy:

1. **Truthful and testable.** Commands, defaults, limits, and error codes match shipped code; each page has tested
   command evidence in `docs/public/owners.json`, and drift checks fail when public surfaces change without docs.
2. **Task-first structure.** Lead with what the reader does; reference tables follow prose explanation. One H1 per
   page; headings describe tasks or contracts, not internals.
3. **Consistent voice.** Present tense, active voice, imperative for instructions. Consistent product terms:
   Control, Egress worker, deployment, runtime snapshot, receipt, trust boundary.
4. **Consistent mechanics.** Bullet lists use one punctuation scheme per list; code identifiers are backticked;
   list items are parallel in form; no missing blank lines around headings or fences.
5. **No internal residue.** User-facing pages do not carry CI/infrastructure implementation detail, references to
   technologies the product does not use, or duplicated paragraphs.
6. **Honest scope.** Every surface states what it does not do, its stability level, and its operator
   responsibilities; examples are labeled adaptable versus supported.
7. **First-contact completeness.** README carries badges, a one-paragraph definition, runnable quickstart, links to
   manual/support/governance/contributing, project status, and license.

## Audit record (2026-07-13)

Scope: `README.md`, `CONTRIBUTING.md`, `SUPPORT.md`, `GOVERNANCE.md`, `SECURITY.md`, `ROADMAP.md`, `CHANGELOG.md`,
all of `docs/public/**`, `deploy/*/README.md`, `examples/README.md`, `scripts/README.md`, `website/README.md`.

Assessment: the corpus already meets or exceeds the bar in structure, truthfulness, contract completeness, scope
honesty, and testing discipline — the readiness program (`open-source-readiness-roadmap.md`) resolved the earlier
structural gaps. The remaining findings were copy-level and were fixed in this audit:

| Finding | Location | Fix |
| --- | --- | --- |
| Mixed bullet punctuation and "receipts" used as a verb | `docs/public/index.md` | consistent semicolons; reworded to "stores … as expiring receipts" |
| Irrelevant technology list (Postgres, ClickHouse) in requirements | `docs/public/quickstart.md` | replaced with "No language toolchain or database is required" |
| CI implementation detail in user quickstart | `docs/public/quickstart.md` | simplified to "exercised by the maintained `make quickstart-smoke` check" |
| Missing blank line before a heading | `docs/public/cli.md` | inserted |
| Stray ClickHouse mention | `docs/public/operations.md` | generalized to "No telemetry database is required" |
| Duplicated "no application database" paragraph | `docs/public/deployment.md` | deduplicated |
| README lacked status badges | `README.md` | added CI, Security, and license badges |
| Landing page lacked project status, license, and support path | `website/src/pages/index.js` | added pre-1.0/MIT status with compatibility and support links |
| Footer labels drifted from page titles; no support/contributing/security/governance/changelog routes | `website/docusaurus.config.js` | aligned labels with page titles; added a Project column |
| Contributor entry page was a thin link hub | `docs/public/development.md` | expanded with setup, repository map, verification loop, and public-contract checklist |

## Keeping the bar

- Review new or changed public pages against the seven criteria above; the pull-request documentation checklist in
  `CONTRIBUTING.md` and the gates (`make docs-check`, `make doc-ownership-check`, `make docs-website`) remain
  mandatory.
- Re-run a copy-level sweep in the quarterly freshness pass defined in `docs/public/documentation-policy.md`.

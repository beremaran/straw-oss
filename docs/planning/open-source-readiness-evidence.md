# Open-source readiness implementation evidence

Date: 2026-07-13. Owner: maintainer. Status: active evidence ledger.

This ledger prevents a narrow green check from being mistaken for launch completion. `pass` means current evidence
directly exercises the acceptance claim; `launch gate` means proof requires intentional publication; `launch QA`
means a prepared workflow must run against published artifacts or live infrastructure; and `owner gate` means a
maintainer-controlled provider or governance action remains.

| Item | Status | Current authoritative evidence / remaining proof |
| --- | --- | --- |
| OR-001 | launch gate | `scripts/verify-clean-room.sh` disables all credentials, anonymously probes all six repositories, clones Straw at the selected ref into a disposable HOME, and runs Go/Python resolution plus `make check`; owner intentionally keeps repositories private until launch |
| OR-002 | pass | owner confirmed the credential's validity is unknowable; context showed only a Bearer header to local Straw and no identifiable provider/account, so no revocation claim was made; owner authorized history rewriting, `test.sh` and all historical `sk_live_...` patterns were removed across local refs and force-pushed remote `main`, stale checkpoint refs/unreachable objects were pruned, and a fresh mirror plus GitHub PR refs scanned clean; `make security-check` rejects patterns in tracked and non-ignored untracked files (proved with a disposable canary); native GitHub scanning/push protection remains a visibility-change launch setting because GitHub returns 422 while private |
| OR-003 | launch gate | identity/toolchain/navigation/link fixes are complete, desktop/mobile production-site rendering and edit links were browser-QA'd, and the full quickstart passed on an owned disposable stack; anonymous container build and published-link proof run after intentional publication |
| OR-004 | launch QA | protected release workflow, launch checklist, compatibility/release/rollback docs, binaries/checksums/CycloneDX, OCI SBOM/provenance/signing exist; `make release-artifact-check` proves reproducible binaries/metadata/checksums/SBOM; an actual protected tagged prerelease and post-publish smoke remain launch gates |
| OR-010 | pass | Control orchestration/assignment/request/decoded/raw/error/conversion files, characterization tests, `make check`, and `make race` |
| OR-011 | pass | Egress resolver/request/pool/validation/error-policy/frame files plus existing DNS/CNAME/redirect/TLS/HTTP2/tunnel conformance and race tests |
| OR-012 | pass | receipt state machine tests, focused create/complete/assignment/cleanup/repository files, record/index repository interface, deterministic receipt tests and race suite |
| OR-013 | pass | `cmd/control/runtime.go`, typed composition boundaries, and `go list` direct-import allowlist in dependency check |
| OR-014 | pass | config/request/admin/receipt/CLI/metrics/compatibility references plus source-derived field/route/error/flag/metric drift gate |
| OR-015 | pass | dead credential/MITM/aliases removed, active-vs-archive planning split, maintained-script policy, runnable examples, canonical `.agents` skill only |
| OR-020 | pass | full/race suites plus bounded config/snapshot, request, DNS/URL/prefix, envelope, frame-sequence, S3 XML, and receipt-record fuzz targets execute successfully; real-service suites preserve sanitized command output |
| OR-021 | pass | versioned conformance manifest, orphan detection, root target, unprivileged public CI, trusted scheduled compatibility workflow, pinned actions/timeouts/concurrency |
| OR-022 | pass | production rendering and a real CA-verified HAProxy TLS request/readiness path passed; live namespaced default/admin/receipt profiles passed; receipt checksum corruption is rejected; the HA drill passed with two workers, Control loss, Redis degradation/recovery, graceful worker loss, and teardown |
| OR-023 | pass | threat model, profile checklists, policy/conformance tests, least-privilege CodeQL/dependency/secret/OCI workflows, Dependabot, zero-finding npm audit, reviewed npm install-script policy, `govulncheck`, pinned images, intended-content verification, SBOM/provenance/signing, and a strict Go/npm license inventory exist; the unlicensed `fhttp`/`tls-client` graph was replaced with locked licensed uTLS/`x/net/http2` primitives while repeated wire, streaming, trailer, cancellation, and maximum-upload conformance tests remain green |
| OR-024 | pass | metrics/logging/SLI/alerts, backup/upgrade guidance, diagnostic runbook matrix and redaction bundle exist; live isolated admin-state and receipt-content backup/restore plus HA failure/recovery drills passed |
| OR-030 | pass | maintained curl/CLI/Go/Python examples pass compile/syntax gates and all four executed successfully against the shipped disposable stack; SDK/custom-worker lifecycle guidance covers the required responsibilities |
| OR-031 | pass | contributor workflow, `make help`, ADRs, compatibility/change criteria, release graph and checks |
| OR-032 | pass | support/governance/security policies, issue/PR templates, ownership/freshness manifest and quarterly gate |
| OR-033 | launch gate | site, heading/link/terminology/fence/code-syntax, ownership, feedback-route, and freshness gates are wired into CI; the built production site passed desktop/mobile browser QA with no console errors; external checks intentionally remain red until publication |

## Locally green evidence

- `make check`
- `make race`
- `make production-deploy-check`
- `make docs-website`
- `actionlint`
- `git diff --check`
- `make fuzz-smoke`
- `make release-artifact-check`
- `make tls-proxy-check`
- `make image-content-check` against locally rebuilt release images
- `make license-check` (50 Go modules and 1,348 npm packages; no missing notices)
- `make npm-audit` (zero vulnerabilities after pinning patched `uuid` 11.1.1)
- live `make quickstart-smoke`, all three `make profile-smoke` profiles, both `make state-backup-smoke` profiles,
  `make ha-smoke`, and `make examples-live` using locally built release-equivalent images
- `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` (zero reachable vulnerabilities)

## Intentionally red evidence

- `make clean-room-check`: anonymous access fails at intentionally private `beremaran/straw-oss`.
- default-build invocations of the Compose smoke targets: anonymous image build cannot resolve private Go tags.
- `make docs-check-external`: public repository, documentation site, protocol, and SDK URLs return 404.

# Security policy

## Supported versions

Straw is pre-1.0. Security fixes are applied to the latest released minor version and the default branch. Older
versions may be asked to upgrade before receiving a fix.

## Report a vulnerability

Do not open a public GitHub issue. Email berke@kwilabs.com with:

- the affected version or commit;
- the component and deployment assumptions;
- reproduction steps or a proof of concept;
- the impact you believe is possible;
- any suggested mitigation.

Do not include real credentials, private traffic, or data belonging to others. You should receive an acknowledgement
within 5 business days. The maintainer will coordinate validation, remediation, disclosure timing, and credit with
you. Please allow a reasonable remediation window before public disclosure.

Automated dependency, CodeQL, vulnerability, secret-pattern, and container scans are release gates. Critical and high
reachable findings block release. A false positive or temporary exception must be documented privately with an
owner, evidence, compensating control, and expiry; broad scan suppressions are not accepted. GitHub secret scanning
and push protection should remain enabled in repository settings. If a credential reaches Git history, revoke or
rotate it first, preserve private incident evidence, and coordinate before considering a history rewrite.

## Deployment responsibility

Straw can originate requests to network destinations. Operators must authenticate and encrypt Control access,
secure NATS, restrict service listeners, manage outbound network policy, and isolate deployments with different
trust requirements. See `docs/public/security.md`.

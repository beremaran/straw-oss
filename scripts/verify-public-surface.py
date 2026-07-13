#!/usr/bin/env python3
"""Fail when source-declared public surfaces or selected normative doc contracts are absent."""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS = "\n".join(path.read_text() for path in (ROOT / "docs/public").rglob("*.md"))
DOC_TEXT = {
    path.relative_to(ROOT).as_posix(): path.read_text()
    for path in (ROOT / "docs/public").rglob("*.md")
}

config_source = (ROOT / "internal/config/config.go").read_text() + (ROOT / "internal/config/snapshot.go").read_text()
route_source = "\n".join(path.read_text() for path in (ROOT / "internal/control").glob("*handler.go")) + (ROOT / "cmd/control/runtime.go").read_text()
flag_source = (ROOT / "internal/cli/cli.go").read_text() + (ROOT / "cmd/control/main.go").read_text() + (ROOT / "cmd/egress/main.go").read_text()
sources = [
    ("JSON field", config_source, r'json:"([^",]+)'),
    ("route", route_source, r'(?:GET|POST|PUT|DELETE) (/[^\s"`]+)'),
    ("metric", (ROOT / "internal/control/metrics.go").read_text(), r'(?:Name:\s*|name:\s*)"([a-z][a-z0-9_]+)"'),
    ("flag", flag_source, r'\.(?:String|Bool|Int|Int64|Uint|Uint64|Duration|Float64)\("([a-z0-9-]+)"'),
    ("flag", flag_source, r'\.Var\([^,]+,\s*"([a-z0-9-]+)"'),
    ("error code", (ROOT / "internal/control/errors.go").read_text(), r'errorCode[A-Za-z0-9]+\s*=\s*"([a-z0-9_]+)"'),
]

missing = []
for kind, source, pattern in sources:
    for value in sorted(set(re.findall(pattern, source))):
        value = value.rstrip(".,;:")
        if value and value not in DOCS:
            missing.append(f"{kind} {value}")
if missing:
    raise SystemExit("public source is absent from docs:\n" + "\n".join(missing))

required_doc_fragments = {
    "docs/public/api/requests.md": [
        "## Validation, defaults, and limits",
        "4 MiB",
        "16,384",
        "total_deadline_timeout",
    ],
    "docs/public/architecture.md": [
        "## Destination policy and egress safety",
        "169.254.169.254",
        "does not follow HTTP redirects",
    ],
    "docs/public/configuration.md": [
        "### Complete runtime snapshot example",
        "### Runtime header injection",
        "allow_override",
        '"op": "append"',
        '"op": "remove"',
    ],
    "docs/public/deployment.md": [
        "### Compose overlay commands",
        "STRAW_TLS_PORT",
        "compose.ha.yml",
    ],
    "docs/public/api/receipts.md": [
        "Authorization: Bearer <request-token>",
        "out of order",
        "part may have size zero",
    ],
    "docs/public/installation.md": [
        "SHA256SUMS",
        "ghcr.io/beremaran/straw-oss-control",
        "gh attestation verify",
    ],
    "docs/public/api/admin.md": [
        "## Examples",
        "revision_conflict",
        "if_match_required",
    ],
    "docs/public/operations.md": [
        "scrape_configs:",
        "straw_workers_available == 0",
        "StrawReceiptRejections",
    ],
    "docs/public/egress_worker.md": [
        "straw.v1.control.register",
        "make conformance",
        "_INBOX.wrk.<worker_id>",
    ],
}

semantic_missing = []
for doc_path, fragments in required_doc_fragments.items():
    text = DOC_TEXT.get(doc_path, "")
    for fragment in fragments:
        if fragment not in text:
            semantic_missing.append(f"{doc_path}: {fragment}")
if semantic_missing:
    raise SystemExit("normative documentation anchors are absent:\n" + "\n".join(semantic_missing))

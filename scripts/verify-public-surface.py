#!/usr/bin/env python3
"""Fail when source-declared public fields, routes, metrics, or flags lack docs."""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS = "\n".join(path.read_text() for path in (ROOT / "docs/public").rglob("*.md"))

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

#!/usr/bin/env python3
"""Validate that every conformance fixture is declared and valid JSON."""

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = ROOT / "conformance"
manifest = json.loads((BASE / "manifest.json").read_text())
schema = json.loads((BASE / "manifest.schema.json").read_text())
if schema.get("properties", {}).get("schema_version", {}).get("const") != 1:
    raise SystemExit("conformance manifest schema must describe version 1")
if manifest.get("schema_version") != 1:
    raise SystemExit("unsupported conformance manifest schema_version")

declared = set()
for entry in manifest.get("fixtures", []):
    path = entry.get("path")
    if not isinstance(path, str) or entry.get("outcome") not in {"accept", "reject"}:
        raise SystemExit(f"invalid fixture entry: {entry!r}")
    target = BASE / path
    if not target.is_file():
        raise SystemExit(f"declared fixture is missing: {path}")
    try:
        json.loads(target.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise SystemExit(f"invalid JSON fixture {path}: {exc}") from exc
    declared.add(path)

present = {str(path.relative_to(BASE)) for path in (BASE / "fixtures").rglob("*.json")}
if present != declared:
    raise SystemExit(f"conformance manifest mismatch; undeclared={sorted(present-declared)}, missing={sorted(declared-present)}")
if not manifest.get("compatibility", {}).get("consumers"):
    raise SystemExit("conformance manifest must name consumers")

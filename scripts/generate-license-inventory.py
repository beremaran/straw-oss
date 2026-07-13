#!/usr/bin/env python3
"""Generate a deterministic dependency-license inventory and fail on missing licenses."""

import hashlib
import json
import subprocess
import sys
from pathlib import Path


def modules():
    subprocess.run(["go", "mod", "download", "all"], check=True)
    raw = subprocess.run(["go", "list", "-m", "-json", "all"], check=True, capture_output=True, text=True).stdout
    decoder = json.JSONDecoder()
    offset = 0
    while offset < len(raw):
        while offset < len(raw) and raw[offset].isspace():
            offset += 1
        if offset == len(raw):
            break
        module, offset = decoder.raw_decode(raw, offset)
        yield module


def detect(text):
    lowered = text.lower()
    if "apache license" in lowered and "version 2.0" in lowered:
        return "Apache-2.0"
    if "permission is hereby granted, free of charge" in lowered:
        return "MIT"
    if "redistribution and use in source and binary forms" in lowered:
        return "BSD-3-Clause" if "neither the name" in lowered else "BSD-2-Clause"
    if "mozilla public license version 2.0" in lowered:
        return "MPL-2.0"
    if "permission to use, copy, modify, and/or distribute" in lowered:
        return "ISC"
    return "UNKNOWN"


def license_files(root):
    return sorted(
        path for path in root.iterdir()
        if path.is_file() and path.name.lower().startswith(("license", "copying", "notice"))
    )


def file_records(root):
    records = []
    for path in license_files(root):
        content = path.read_bytes()
        records.append({
            "file": path.name,
            "sha256": hashlib.sha256(content).hexdigest(),
            "detected_spdx": detect(content.decode(errors="replace")),
        })
    return records


output = Path(sys.argv[1]) if len(sys.argv) == 2 else Path("dist/dependency-licenses.json")
go_records = []
failures = []
for module in modules():
    directory = module.get("Dir")
    if not directory:
        failures.append(f"{module['Path']} has no downloaded module directory")
        continue
    root = Path(directory)
    licenses = file_records(root)
    if not licenses:
        failures.append(f"{module['Path']}@{module.get('Version', 'main')} has no distributed license/notice file")
    elif not any(item["detected_spdx"] != "UNKNOWN" for item in licenses):
        failures.append(f"{module['Path']}@{module.get('Version', 'main')} has notices but no recognized license text")
    go_records.append({
        "module": module["Path"],
        "version": module.get("Version", "main"),
        "licenses": licenses,
    })

lock_path = Path("website/package-lock.json")
modules_path = Path("website/node_modules")
if not lock_path.is_file() or not modules_path.is_dir():
    failures.append("website dependencies are not installed; run npm ci in website")
    npm_records = []
else:
    lock = json.loads(lock_path.read_text())
    npm_records = []
    for package_path, metadata in sorted(lock.get("packages", {}).items()):
        if not package_path.startswith("node_modules/"):
            continue
        name = package_path.removeprefix("node_modules/")
        root = Path("website") / package_path
        licenses = file_records(root) if root.is_dir() else []
        declared = metadata.get("license") or metadata.get("licenses")
        package_json = root / "package.json"
        if not declared and package_json.is_file():
            installed_metadata = json.loads(package_json.read_text())
            declared = installed_metadata.get("license") or installed_metadata.get("licenses")
        if not declared and not licenses:
            failures.append(f"npm:{name}@{metadata.get('version', 'unknown')} has no license metadata or distributed license/notice file")
        npm_records.append({
            "package": name,
            "version": metadata.get("version", "unknown"),
            "declared_license": declared,
            "licenses": licenses,
        })

go_records.sort(key=lambda record: record["module"])
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps({
    "schema_version": 1,
    "go_modules": go_records,
    "npm_packages": npm_records,
}, indent=2, sort_keys=True) + "\n")
if failures:
    raise SystemExit("license inventory failed:\n" + "\n".join(failures))
print(f"license inventory: {len(go_records)} Go modules and {len(npm_records)} npm packages, no missing notices")

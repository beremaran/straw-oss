#!/usr/bin/env python3
"""Validate Markdown structure, local links, terminology, and optional external links."""

import argparse
import json
import re
import subprocess
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
FILES = list((ROOT / "docs/public").rglob("*.md")) + [ROOT / name for name in ("README.md", "CONTRIBUTING.md", "SECURITY.md", "SUPPORT.md", "GOVERNANCE.md")]
LINK = re.compile(r"(?<!!)\[[^]]+\]\(([^)]+)\)")
failures = []
external = set()
allowed_fences = {"sh", "bash", "json", "text", "mermaid", "go", "python"}
common_misspellings = {
    "compatability", "configuraton", "dependancy", "occured", "recieve", "reproducable", "seperate", "succesful", "teh",
}

for path in FILES:
    text = path.read_text()
    relative = path.relative_to(ROOT)
    if len(re.findall(r"^# ", text, re.MULTILINE)) != 1:
        failures.append(f"{relative}: expected exactly one level-one heading")
    previous_heading = 0
    in_fence = False
    fence_language = ""
    fence_start = 0
    fence_lines = []
    table_rows = []
    for line_number, line in enumerate(text.splitlines(), 1):
        if line.rstrip() != line:
            failures.append(f"{relative}:{line_number}: trailing whitespace")
        if "\t" in line:
            failures.append(f"{relative}:{line_number}: tab character")
        if line.startswith("```"):
            if not in_fence:
                fence_language = line[3:].strip()
                fence_start = line_number
                fence_lines = []
                in_fence = True
                if fence_language not in allowed_fences:
                    failures.append(f"{relative}:{line_number}: missing or unsupported fence language {fence_language!r}")
            else:
                if line != "```":
                    failures.append(f"{relative}:{line_number}: closing fence must not carry a language")
                snippet = "\n".join(fence_lines) + "\n"
                try:
                    if fence_language in {"sh", "bash"}:
                        result = subprocess.run(["bash", "-n"], input=snippet, text=True, capture_output=True)
                        if result.returncode:
                            failures.append(f"{relative}:{fence_start}: invalid shell example: {result.stderr.strip()}")
                    elif fence_language == "json":
                        json.loads(snippet)
                    elif fence_language == "python":
                        compile(snippet, f"{relative}:{fence_start}", "exec")
                    elif fence_language == "go":
                        result = subprocess.run(["gofmt"], input=snippet, text=True, capture_output=True)
                        if result.returncode:
                            failures.append(f"{relative}:{fence_start}: invalid Go example: {result.stderr.strip()}")
                except (json.JSONDecodeError, SyntaxError) as exc:
                    failures.append(f"{relative}:{fence_start}: invalid {fence_language} example: {exc}")
                in_fence = False
            continue
        if in_fence:
            fence_lines.append(line)
            continue
        heading = re.match(r"^(#{1,6}) ", line)
        if heading:
            level = len(heading.group(1))
            if previous_heading and level > previous_heading + 1:
                failures.append(f"{relative}:{line_number}: heading level jumps from {previous_heading} to {level}")
            previous_heading = level
        table_rows.append((line_number, line) if line.lstrip().startswith("|") else None)
    if in_fence:
        failures.append(f"{relative}:{fence_start}: unclosed fenced code block")
    # A GFM table row splits on every unescaped pipe, including pipes inside code spans. A row whose cell count
    # disagrees with its header renders phantom columns, so require an explicit \| for any literal pipe.
    columns = None
    for entry in table_rows + [None]:
        if entry is None:
            columns = None
            continue
        line_number, line = entry
        cells = len(re.findall(r"(?<!\\)\|", line.strip())) - 1
        if columns is None:
            columns = cells
            continue
        if set(line.strip()) <= set("|-: "):
            continue
        if cells != columns:
            failures.append(f"{relative}:{line_number}: table row has {cells} cells but the header declares {columns}; escape any literal pipe as \\|")
    words = {word.lower() for word in re.findall(r"[A-Za-z]+", text)}
    for misspelling in sorted(words & common_misspellings):
        failures.append(f"{relative}: common misspelling {misspelling!r}")
    for target in LINK.findall(text):
        target = target.strip("<>")
        if target.startswith(("http://", "https://")):
            external.add(target)
            continue
        if target.startswith(("mailto:", "#")):
            continue
        local = target.split("#", 1)[0]
        if not local:
            continue
        resolved = (path.parent / local).resolve()
        if not resolved.exists():
            failures.append(f"{relative}: broken local link {target}")
    for stale in ("github.com/beremaran/straw-oss/blob/master", "github.com/beremaran/straw-oss/tree/master", "exact private Git tag", "private Python SDK"):
        if stale in text:
            failures.append(f"{relative}: stale terminology {stale!r}")

parser = argparse.ArgumentParser()
parser.add_argument("--external", action="store_true")
args = parser.parse_args()
if args.external:
    for url in sorted(external):
        request = urllib.request.Request(url, headers={"User-Agent": "straw-doc-check/1"}, method="HEAD")
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                if response.status >= 400:
                    failures.append(f"external link returned {response.status}: {url}")
        except (urllib.error.URLError, TimeoutError) as exc:
            failures.append(f"external link failed: {url}: {exc}")

if failures:
    raise SystemExit("documentation verification failed:\n" + "\n".join(failures))

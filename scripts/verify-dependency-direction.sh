#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

test ! -e api/proto
test ! -e sdk
test ! -e python
test ! -e examples/egress-static
test -z "$(git ls-files 'go.work' '**/go.work')"
! grep -Eq '^[[:space:]]*replace[[:space:](]' go.mod
grep -Eq 'github.com/beremaran/straw-protos-go v0\.3\.0$' go.mod
grep -Eq 'github.com/beremaran/straw-sdk-go v0\.1\.0$' go.mod
grep -Fq 'straw-sdk-python.git@v0.1.0' pyproject.toml
grep -Fq 'straw-sdk-python.git?rev=v0.1.0#' uv.lock
grep -Fq 'straw-protos-python.git?rev=v0.3.0#' uv.lock
! grep -Eq 'source = \{ (editable|directory|path) =' uv.lock

if rg -n 'github.com/beremaran/straw-oss/(sdk|api/proto)' --glob '*.go' .; then
  echo 'found a removed monorepo package import' >&2
  exit 1
fi

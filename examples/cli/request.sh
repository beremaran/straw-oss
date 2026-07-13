#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
binary=$(mktemp "${TMPDIR:-/tmp}/straw-cli-example.XXXXXX")
trap 'rm -f "$binary"' EXIT
go build -o "$binary" "$root/cmd/straw"
"$binary" request --url https://example.com --timeout-ms 15000

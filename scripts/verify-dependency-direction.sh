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
grep -Eq 'github.com/beremaran/straw-sdk-go v0\.2\.0$' go.mod
grep -Eq 'github.com/bogdanfinn/utls v1\.7\.7-barnius$' go.mod
grep -Eq 'golang.org/x/net v0\.56\.0$' go.mod
! grep -Eq 'github.com/bogdanfinn/(fhttp|tls-client)' go.mod
grep -Fq 'straw-sdk-python.git@v0.2.0' pyproject.toml
grep -Fq 'straw-sdk-python.git?rev=v0.2.0#' uv.lock
grep -Fq 'straw-protos-python.git?rev=v0.3.0#' uv.lock
! grep -Eq 'source = \{ (editable|directory|path) =' uv.lock

if rg -n 'github.com/beremaran/straw-oss/(sdk|api/proto)' --glob '*.go' .; then
  echo 'found a removed monorepo package import' >&2
  exit 1
fi

# Enforce direct internal imports for the public runtime graph. Standard-library
# and third-party imports are intentionally outside this architectural rule.
module=github.com/beremaran/straw-oss
check_internal_imports() {
  package=$1
  allowed=$2
  imports=$(go list -f '{{range .Imports}}{{println .}}{{end}}' "$package" | grep "^$module/internal/" || true)
  while IFS= read -r imported; do
    test -z "$imported" && continue
    case " $allowed " in
      *" $imported "*) ;;
      *) echo "$package imports forbidden package $imported" >&2; return 1 ;;
    esac
  done <<<"$imports"
}

check_internal_imports ./cmd/control "$module/internal/config $module/internal/control $module/internal/fingerprint $module/internal/logging $module/internal/natsx $module/internal/objectstore $module/internal/receipt"
check_internal_imports ./cmd/egress "$module/internal/config $module/internal/egress $module/internal/logging $module/internal/natsx"
check_internal_imports ./cmd/straw "$module/internal/cli"
check_internal_imports ./internal/control "$module/internal/config $module/internal/fingerprint $module/internal/natsx $module/internal/receipt"
check_internal_imports ./internal/egress "$module/internal/egress/profilecatalog $module/internal/fingerprint $module/internal/natsx"
check_internal_imports ./internal/egress/profilecatalog ""
check_internal_imports ./internal/fingerprint ""
check_internal_imports ./internal/config ""
check_internal_imports ./internal/natsx ""
check_internal_imports ./internal/receipt "$module/internal/objectstore"
check_internal_imports ./internal/objectstore ""

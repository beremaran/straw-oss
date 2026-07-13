#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

pattern='(sk_live_[A-Za-z0-9]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----)'
found=false
if git grep -nEI "$pattern" -- ':!scripts/verify-secrets.sh'; then
  found=true
fi

while IFS= read -r -d '' file; do
  test "$file" = scripts/verify-secrets.sh && continue
  if rg -n -I -e "$pattern" -- "$file"; then
    found=true
  fi
done < <(git ls-files --others --exclude-standard -z)

if "$found"; then
  echo 'credential-like material found in tracked or non-ignored untracked files' >&2
  echo 'Replace false positives with non-secret fixtures; do not add broad suppressions.' >&2
  exit 1
fi

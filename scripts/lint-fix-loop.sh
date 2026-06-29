#!/usr/bin/env bash

set -euo pipefail

LINT_FLAGS=(--max-issues-per-linter 0 --max-same-issues 0)
CHECKPOINT_DIR=".lint-checkpoints/$(date +%Y%m%d-%H%M%S)"
agent=opencode
checkpoint_count=0
iteration=0

while (($#)); do
  case "$1" in
    --codex)
      agent=codex
      ;;
    -h | --help)
      printf "Usage: %s [--codex]\n" "${0##*/}"
      exit 0
      ;;
    *)
      printf "unknown option: %s\n" "$1" >&2
      exit 2
      ;;
  esac
  shift
done

print_target_header() {
  local target_index=$1
  local target_total=$2
  local target=$3

  printf '\n\n============================================================\n' >&2
  printf '[iteration %d] package %d/%d: %s\n' "$iteration" "$target_index" "$target_total" "$target" >&2
  printf '============================================================\n\n\n' >&2
}

checkpoint() {
  if git diff --quiet -- '*.go'; then
    return
  fi

  mkdir -p "$CHECKPOINT_DIR"
  checkpoint_count=$((checkpoint_count + 1))
  git diff --binary -- '*.go' > "$CHECKPOINT_DIR/$(printf '%04d' "$checkpoint_count").patch"
}

fix_lint_errors() {
  local target=$1
  local errors=$2
  local prompt

  prompt="Fix only the lint errors for $target. Do not modify .golangci.yml. Do not add nolint comments.

Lint output:
${errors}"

  if [[ "$agent" == codex ]]; then
    codex exec --model gpt-5.4-mini --dangerously-bypass-hook-trust --dangerously-bypass-approvals-and-sandbox "$prompt"
    return
  fi

  opencode run \
    --title "fixing linter errors: $target" \
    --thinking \
    "$prompt"
}

while ! make lint >/dev/null 2>&1; do
  iteration=$((iteration + 1))
  found_errors=false
  targets=()

  # ponytail: lint package directories so golangci-lint can resolve sibling types.
  while IFS= read -r -d '' file; do
    if [[ "$file" == */* ]]; then
      target=${file%/*}
    else
      target=.
    fi

    targets+=("$target")
  done < <(find . -name '*.go' -not -path './vendor/*' -not -path './.git/*' -print0)

  unique_targets=()
  while IFS= read -r target; do
    unique_targets+=("$target")
  done < <(printf '%s\n' "${targets[@]}" | sort -u)

  target_total=${#unique_targets[@]}
  target_index=0

  for target in "${unique_targets[@]}"; do
    target_index=$((target_index + 1))
    print_target_header "$target_index" "$target_total" "$target"

    if errors=$(golangci-lint run "${LINT_FLAGS[@]}" "$target" 2>&1); then
      continue
    fi

    found_errors=true
    checkpoint

    if ! fix_lint_errors "$target" "$errors" </dev/null; then
      printf "%s failed while fixing %s\n" "$agent" "$target" >&2
      exit 1
    fi

    checkpoint
  done

  if [[ "$found_errors" == false ]]; then
    make lint >/dev/null 2>&1
  fi
done

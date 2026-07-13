#!/usr/bin/env bash
set -Eeuo pipefail

control_image=${STRAW_CONTROL_IMAGE:-straw-control:readiness}
egress_image=${STRAW_EGRESS_IMAGE:-straw-egress:readiness}

expected_context=$'**\n!go.mod\n!go.sum\n!cmd/\n!cmd/**\n!internal/\n!internal/**\n!LICENSE\n!THIRD_PARTY_NOTICES.md'
[[ $(<.dockerignore) == "$expected_context" ]] || {
  echo '.dockerignore must remain an explicit production-build allowlist' >&2
  exit 1
}

ensure_image() {
  local component=$1 image=$2
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build -f "deploy/docker/Dockerfile.$component" -t "$image" .
  fi
}

verify_image() {
  local component=$1 image=$2 title=$3 container listing
  [[ $(docker image inspect --format '{{.Config.User}}' "$image") == 65532 ]]
  [[ $(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.source"}}' "$image") == https://github.com/beremaran/straw-oss ]]
  local expected_licenses=MIT
  [[ $component == egress ]] && expected_licenses='MIT AND BSD-4-Clause'
  [[ $(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.licenses"}}' "$image") == "$expected_licenses" ]]
  [[ $(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.title"}}' "$image") == "$title" ]]
  docker image inspect --format '{{json .Config.Entrypoint}}' "$image" | grep -q "^\[\"/$component\",\"-config\",\"/etc/straw/$component.json\"\]$"
  docker image inspect --format '{{json .Config.Healthcheck.Test}}' "$image" | grep -q "^\[\"CMD\",\"/$component\",\"-healthcheck\"\]$"

  if docker run --rm --entrypoint /bin/sh "$image" -c true >/dev/null 2>&1; then
    echo "$image unexpectedly contains a shell" >&2
    return 1
  fi

  container=$(docker create "$image")
  trap 'docker rm -f "$container" >/dev/null 2>&1 || true' RETURN
  listing=$(docker export "$container" | tar -tf -)
  grep -Fxq "$component" <<<"$listing"
  if [[ $component == egress ]]; then
    grep -Fxq THIRD_PARTY_NOTICES.md <<<"$listing"
  fi
  if printf '%s\n' "$listing" | grep -v '/$' | grep -E '(^|/)(\.git|go|src|workspace|[^/]*secret[^/]*|[^/]*token[^/]*)($|/)' >/dev/null; then
    echo "$image contains build source, toolchain, or secret-named paths" >&2
    return 1
  fi
  docker rm "$container" >/dev/null
  trap - RETURN
}

ensure_image control "$control_image"
ensure_image egress "$egress_image"
verify_image control "$control_image" 'Straw Control'
verify_image egress "$egress_image" 'Straw Egress'
echo 'release image contents: non-root, labelled, health-aware, shell-free, and source-free'

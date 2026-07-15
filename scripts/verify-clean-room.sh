#!/usr/bin/env bash
set -euo pipefail

workspace=$(mktemp -d)
cleanup() {
  # Go and uv may create read-only cache entries under the isolated HOME.
  # Make the temporary workspace writable before removing it on exit.
  chmod -R u+w "$workspace" 2>/dev/null || true
  rm -rf "$workspace"
}
trap cleanup EXIT
export HOME="$workspace/home"
mkdir -p "$HOME"
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0
unset GOPRIVATE GONOSUMDB GOPROXY UV_INDEX UV_EXTRA_INDEX_URL GH_TOKEN GITHUB_TOKEN

for repository in straw-oss straw-protos straw-protos-go straw-sdk-go straw-protos-python straw-sdk-python; do
  git -c credential.helper= ls-remote "https://github.com/beremaran/${repository}.git" HEAD </dev/null >/dev/null || {
    echo "public dependency is not anonymously readable: beremaran/${repository}" >&2
    exit 1
  }
done

ref=${CLEAN_ROOM_REF:-main}
git -c credential.helper= clone --quiet --depth 1 --branch "$ref" https://github.com/beremaran/straw-oss.git "$workspace/straw-oss" </dev/null
cd "$workspace/straw-oss"
go mod download
uv sync --frozen --refresh
make check

#!/usr/bin/env bash
set -Eeuo pipefail

version=$(awk '$1 == "go" {print $2; exit}' go.mod)
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
for dockerfile in deploy/docker/Dockerfile.control deploy/docker/Dockerfile.egress; do
  grep -q "^FROM golang:${version}@sha256:" "$dockerfile"
done
grep -q "Go ${version} (the exact version declared by \`go.mod\`)" CONTRIBUTING.md
grep -q "| Go / Python / Node | ${version} /" docs/public/compatibility.md
grep -q "Go ${version}, Python" examples/README.md
echo "toolchain policy aligned at Go ${version}"

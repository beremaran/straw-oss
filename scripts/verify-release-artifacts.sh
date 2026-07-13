#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export CGO_ENABLED=0
for pass in first second; do
  mkdir -p "$tmp/$pass"
  for os in linux darwin; do
    for arch in amd64 arm64; do
      for command in control egress straw; do
        artifact="straw-${command}_${os}_${arch}"
        GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=true -ldflags='-s -w' -o "$tmp/$pass/$artifact" "$root/cmd/$command"
        go version -m "$tmp/$pass/$artifact" > "$tmp/$pass/$artifact.modules"
      done
    done
  done
done
for artifact in "$tmp/first"/straw-*_*_*; do
  name=${artifact##*/}
  [[ $name == *.modules ]] && continue
  cmp "$artifact" "$tmp/second/$name"
  test -s "$tmp/first/$name.modules"
done
go run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.9.0 mod -json -output "$tmp/straw.cdx.json"
grep -q '"bomFormat": "CycloneDX"' "$tmp/straw.cdx.json"
cp "$root/THIRD_PARTY_NOTICES.md" "$tmp/first/THIRD_PARTY_NOTICES.md"
grep -q 'tls-client profile catalogue' "$tmp/first/THIRD_PARTY_NOTICES.md"
(cd "$tmp/first" && find . -maxdepth 1 -type f -print0 | sort -z | xargs -0 shasum -a 256 > "$tmp/SHA256SUMS")
test -s "$tmp/SHA256SUMS"
echo 'release artifact dry run: reproducible binaries, notices, module metadata, checksums, and SBOM passed'

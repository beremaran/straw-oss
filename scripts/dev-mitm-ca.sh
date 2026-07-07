#!/usr/bin/env sh
set -eu

out_dir="${1:-.dev/mitm-ca}"
days="${STRAW_MITM_CERT_VALIDITY_DAYS:-30}"

mkdir -p "$out_dir"
umask 077

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$out_dir/ca-key.pem" \
  -out "$out_dir/ca.pem" \
  -days "$days" \
  -subj "/CN=Straw Dev MITM CA"

cat <<EOF
Wrote dev/test-only MITM CA material:
  $out_dir/ca.pem
  $out_dir/ca-key.pem

Do not use this helper for production CA material.
EOF

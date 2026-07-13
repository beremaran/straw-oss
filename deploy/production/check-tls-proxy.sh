#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tmp=$(mktemp -d)
suffix="$$"
network="straw-tls-check-$suffix"
backend="straw-tls-backend-$suffix"
proxy="straw-tls-proxy-$suffix"
cleanup() {
  docker rm -f "$proxy" "$backend" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=localhost' -addext 'subjectAltName=DNS:localhost' -keyout "$tmp/key.pem" -out "$tmp/cert.pem" >/dev/null 2>&1
cat "$tmp/cert.pem" "$tmp/key.pem" > "$tmp/straw.pem"
docker run --rm \
  -v "$root/deploy/production/haproxy.tls.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" \
  -v "$tmp/straw.pem:/run/secrets/straw.pem:ro" \
  haproxy:3.2-alpine@sha256:66e25cc9a8332635f4e897f7f4b1e5622c25f09f0ee23cddc6ce9bdb3a24772a \
  haproxy -c -f /usr/local/etc/haproxy/haproxy.cfg

docker network create "$network" >/dev/null
docker run -d --name "$backend" --network "$network" --network-alias control \
  alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce \
  sh -c 'while true; do printf "HTTP/1.1 200 OK\r\nContent-Length: 12\r\nConnection: close\r\n\r\nstraw-tls-ok" | nc -l -p 8080; done & while true; do printf "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nConnection: close\r\n\r\nready" | nc -l -p 9090; done' >/dev/null
docker run -d --name "$proxy" --network "$network" -p '127.0.0.1::8443' --read-only --tmpfs /tmp \
  --security-opt no-new-privileges:true \
  -v "$root/deploy/production/haproxy.tls.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" \
  -v "$tmp/straw.pem:/run/secrets/straw.pem:ro" \
  haproxy:3.2-alpine@sha256:66e25cc9a8332635f4e897f7f4b1e5622c25f09f0ee23cddc6ce9bdb3a24772a >/dev/null

port=$(docker port "$proxy" 8443/tcp | sed -n 's/.*://p')
for _ in {1..30}; do
  if [[ $(curl --silent --show-error --cacert "$tmp/cert.pem" --max-time 2 "https://localhost:$port/" 2>/dev/null || true) == straw-tls-ok ]]; then
    echo 'TLS proxy syntax, readiness routing, certificate, and request path: passed'
    exit 0
  fi
  sleep 1
done
docker logs "$proxy" >&2
echo 'TLS proxy did not become ready' >&2
exit 1

#!/bin/bash
# Pre-flight port availability check for the standalone Straw stack.
set -euo pipefail

PORTS=(4222 8222 5432 6379 8123 8080 8083 9090 3000 3001 9091)
CONFLICTS=()

for port in "${PORTS[@]}"; do
  if nc -z -w 1 127.0.0.1 "$port" >/dev/null 2>&1; then
    CONFLICTS+=("$port")
  fi
done

if [ ${#CONFLICTS[@]} -ne 0 ]; then
  echo "Error: The following required port(s) are already in use on 127.0.0.1:" >&2
  for port in "${CONFLICTS[@]}"; do
    echo "  - $port" >&2
  done
  echo "Please stop the conflicting services and try again." >&2
  exit 1
fi

echo "All required ports are available."
exit 0

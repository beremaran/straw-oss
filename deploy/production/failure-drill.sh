#!/bin/sh
set -eu

compose="docker compose --env-file deploy/production/.env -f deploy/production/compose.ha.yml"
case "${1:-}" in
  control-loss)
    $compose stop control-1
    trap '$compose start control-1' EXIT
    passed=false
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      if curl -fsS -H "Authorization: Bearer ${STRAW_AUTH_TOKEN:?export STRAW_AUTH_TOKEN}" -H 'Content-Type: application/json' \
        -d '{"method":"GET","url":"https://example.com"}' http://127.0.0.1:${STRAW_CONTROL_API_PORT:-8080}/api/v1/requests >/dev/null; then
        passed=true
        break
      fi
      sleep 1
    done
    test "$passed" = true
    ;;
  redis-outage)
    $compose pause redis
    trap '$compose unpause redis' EXIT
    sleep 6
    test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:9091/readyz)" = 503
    test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:9092/readyz)" = 503
    ;;
  *) echo "usage: $0 control-loss|redis-outage" >&2; exit 2 ;;
esac

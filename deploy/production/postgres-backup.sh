#!/bin/sh
set -eu

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
export PGPASSWORD="$POSTGRES_PASSWORD"
pg_dump -h postgres -U "$POSTGRES_USER" "$POSTGRES_DB" >"/backups/${POSTGRES_DB}-${stamp}.sql"
find /backups -type f -name "${POSTGRES_DB}-*.sql" -mtime +"${BACKUP_RETENTION_DAYS:-7}" -delete

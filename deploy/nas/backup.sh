#!/bin/sh
set -eu

compose_file=${COMPOSE_FILE:-compose.yml}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
backup_name="mvforge-${timestamp}.db"

docker compose --env-file .env -f "$compose_file" exec -T backend \
  sh -c 'db=/app/data/mvforge.db; [ -f "$db" ] || db=/app/data/mediaforge.db; mkdir -p /app/data/backups && sqlite3 "$db" ".backup /app/data/backups/$1"' \
  sh "$backup_name"

echo "Backup created under CONFIG_PATH/backups/$backup_name"

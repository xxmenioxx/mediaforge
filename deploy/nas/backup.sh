#!/bin/sh
set -eu

compose_file=${COMPOSE_FILE:-compose.yml}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
backup_name="mediaforge-${timestamp}.db"

docker compose --env-file .env -f "$compose_file" exec -T backend \
  sh -c 'mkdir -p /app/data/backups && sqlite3 /app/data/mediaforge.db ".backup /app/data/backups/$1"' \
  sh "$backup_name"

echo "Backup created under CONFIG_PATH/backups/$backup_name"

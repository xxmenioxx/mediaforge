#!/usr/bin/env sh
set -eu

if [ "${CONFIRM_MVFORGE_RESET:-}" != "YES" ]; then
  echo "Refusing to reset MVForge."
  echo "This deletes the SQLite Docker volume and local media/raw, media/library, media/staging, and media/originals_archive."
  echo "It preserves media/reports so logs, AS-IS reports, and result reports remain available."
  echo ""
  echo "Run with:"
  echo "  CONFIRM_MVFORGE_RESET=YES sh scripts/reset-v1-preserve-reports.sh"
  exit 1
fi

PROJECT_NAME="${COMPOSE_PROJECT_NAME:-mvforge}"
DB_VOLUME="${PROJECT_NAME}_backend-data"

echo "Stopping MVForge containers..."
docker compose down

echo "Removing SQLite database volume: ${DB_VOLUME}"
docker volume rm "${DB_VOLUME}" 2>/dev/null || true

echo "Cleaning local working media folders while preserving reports..."
rm -rf media/raw media/library media/staging media/originals_archive
mkdir -p media/raw media/library media/staging media/originals_archive media/reports/as-is media/reports/results media/reports/logs
touch media/.gitkeep

echo "Starting MVForge with V1 seed data..."
docker compose up -d --build

echo "Done. Open http://localhost:5173"

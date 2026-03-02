#!/bin/sh
set -eu

STATIC_SOURCE_DIR="${STATIC_SOURCE_DIR:-/app/app/static_seed}"
STATIC_TARGET_DIR="${STATIC_TARGET_DIR:-/app/static-data}"
DB_WAIT_TIMEOUT="${DB_WAIT_TIMEOUT:-30}"
DB_PORT="${DB_PORT:-5432}"

mkdir -p "$STATIC_TARGET_DIR"

if [ -z "$(ls -A "$STATIC_TARGET_DIR" 2>/dev/null)" ]; then
  cp -R "$STATIC_SOURCE_DIR"/. "$STATIC_TARGET_DIR"/
fi

echo "Waiting for database at ${DB_HOST}:${DB_PORT}..."
attempt=0
until pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge "$DB_WAIT_TIMEOUT" ]; then
    echo "Database is not ready in time."
    exit 1
  fi
  sleep 1
done

echo "Running database migrations..."
flask db upgrade

echo "Starting application on port ${PORT:-8080}..."
exec gunicorn --bind "0.0.0.0:${PORT:-8080}" --workers "${GUNICORN_WORKERS:-2}" "wsgi:app"

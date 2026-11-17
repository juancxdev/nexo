#!/usr/bin/env bash
set -euo pipefail

project="nexo-events-ci-${GITHUB_RUN_ID:-local}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export EVENT_CONFIGURATION_PORT="${EVENT_CONFIGURATION_PORT:-13000}"
compose=(docker compose -p "$project" -f docker-compose.yml)

cleanup() {
  "${compose[@]}" logs --no-color || true
  "${compose[@]}" down --volumes --remove-orphans || true
}
trap cleanup EXIT

"${compose[@]}" config --quiet
"${compose[@]}" up -d --build --wait --wait-timeout 120 \
  postgres bootstrap-events migrate-events seed-events \
  event-configuration-api event-configuration-worker

"${compose[@]}" exec -T postgres sh -ec '
  export PGPASSWORD=configuration-app-local-only
  psql \
    "postgres://configuration_app:${PGPASSWORD}@localhost:5432/nexo?sslmode=disable" \
    -v ON_ERROR_STOP=1 -Atc "SET search_path TO events, public; SELECT current_database(), current_schema(), rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user; SELECT count(*) FROM events.events;"
' | grep -Eq '^nexo\|events\|f\|f$'

"${compose[@]}" run --rm migrate-events

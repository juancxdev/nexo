#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

dc=(docker compose -f docker-compose.yml)
init=(docker compose -f docker-compose.yml -f docker-compose.init.yml)

"${dc[@]}" up -d --wait --wait-timeout 120 postgres

for job in migrate-core migrate-onsite seed-onsite migrate-operations bootstrap-events migrate-events seed-events seed-operations bootstrap-runtime-roles; do
  "${init[@]}" run --rm --no-deps "$job"
done

"${dc[@]}" up -d --build core-api onsite-operations-api event-operations-api event-configuration-api event-configuration-worker mailpit minio hukid-simulator
"${dc[@]}" ps

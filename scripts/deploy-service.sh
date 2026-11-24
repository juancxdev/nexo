#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

dc=(docker compose -f docker-compose.yml)
init=(docker compose -f docker-compose.yml -f docker-compose.init.yml)

usage() {
  cat <<'EOF'
Uso: ./scripts/deploy-service.sh <core|onsite|participation|operations|events|hukid>

Reconstruye únicamente el servicio seleccionado. Nunca ejecuta seeds ni baja
contenedores ajenos. Las migraciones del dominio se aplican antes de recrear
su API/worker.
EOF
}

wait_for_postgres() {
  "${dc[@]}" up -d postgres
  for _ in $(seq 1 30); do
    if "${dc[@]}" exec -T postgres pg_isready -U nexo -d nexo >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "PostgreSQL no quedó listo a tiempo." >&2
  exit 1
}

target="${1:-}"
case "$target" in
  core)
    wait_for_postgres
    "${init[@]}" run --rm --no-deps migrate-core
    "${dc[@]}" up -d --build --no-deps core-api
    ;;
  onsite)
    wait_for_postgres
    "${init[@]}" run --rm --no-deps migrate-onsite
    "${dc[@]}" up -d --build --no-deps onsite-operations-api
    ;;
  participation)
    wait_for_postgres
    "${init[@]}" run --rm --no-deps migrate-participation
    "${dc[@]}" up -d --build --no-deps participation-api
    ;;
  operations)
    wait_for_postgres
    "${init[@]}" run --rm --no-deps migrate-operations
    "${dc[@]}" up -d --build --no-deps event-operations-api
    ;;
  events)
    wait_for_postgres
    "${init[@]}" run --rm --no-deps migrate-events
    "${dc[@]}" up -d --build --no-deps event-configuration-api event-configuration-worker
    ;;
  hukid)
    "${dc[@]}" up -d --build --no-deps hukid-simulator
    ;;
  -h|--help|help|"")
    usage
    ;;
  *)
    echo "Servicio desconocido: $target" >&2
    usage >&2
    exit 2
    ;;
esac

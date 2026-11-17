#!/usr/bin/env bash

set -Eeuo pipefail

shopt -s nullglob

services=(microservices/*/go.mod)
if ((${#services[@]} == 0)); then
  echo "No se encontraron microservicios Go en microservices/"
  exit 1
fi

for mod in "${services[@]}"; do
  service_dir="${mod%/go.mod}"
  echo "::group::Validando ${service_dir}"

  (
    cd "${service_dir}"

    # Cada servicio puede levantar recursos efímeros propios para sus pruebas.
    # El cleanup evita que un servicio contamine el siguiente.
    cleanup() {
      if [[ -f docker-compose.yml || -f compose.yml ]]; then
        docker compose down -v --remove-orphans >/dev/null 2>&1 || true
      fi
    }
    trap cleanup EXIT

    if [[ -f Makefile ]] && grep -qE '^test:' Makefile; then
      make test
    else
      go test ./...
    fi

    go vet ./...
  )

  echo "::endgroup::"
done

echo "::group::Validando tools/hukid-simulator"
(
  cd tools/hukid-simulator
  go test ./...
  go vet ./...
)
echo "::endgroup::"

event_configuration_dir="microservices/event-configuration"
if [[ -f "${event_configuration_dir}/package.json" ]]; then
  echo "::group::Validando ${event_configuration_dir}"
  (
    cd "${event_configuration_dir}"
    if command -v corepack >/dev/null 2>&1; then
      corepack enable
      corepack prepare pnpm@11.20.0 --activate
    elif ! command -v pnpm >/dev/null 2>&1; then
      echo "pnpm or Corepack is required to validate Event Configuration" >&2
      exit 1
    fi
    pnpm install --frozen-lockfile
    pnpm lint
    pnpm build:api
    pnpm test:api
    pnpm typecheck:contract-consumer
    pnpm test:contract-consumer
    pnpm test:composition
    pnpm test:worker
    pnpm test:event-schemas
    pnpm format:check
    pnpm build:admin
  )
  echo "::endgroup::"
fi

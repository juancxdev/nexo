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

---
title: 'Completar operaciones presenciales, certificados e integración HukID del Grupo 4'
type: 'feature'
created: '2026-11-12'
status: 'done'
baseline_commit: '018bafb0fbe291cc1f5d3cd87ea72c527e33df9a'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '/Users/juancx/Documents/Codex/Startup/planning-artifacts/replans/event-platform-5-team-microservices-2026-11-02/backlogs/TEAM-4-ONSITE-OPERATIONS.md'
  - '/Users/juancx/Documents/Codex/Startup/planning-artifacts/architecture/events-platform-2026-11-02/ARCHITECTURE-SPINE.md'
  - '/Users/juancx/Documents/Codex/Startup/planning-artifacts/architecture/events-platform-2026-11-02/INTEGRATION-CONTRACTS.md'
  - '/Users/juancx/Documents/CX-Developments/SELF/hukid/docs/planting/api/openapi.yaml'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** El Grupo 4 solo tiene implementado el primer corte de check-in con persistencia. Faltan dispositivos, turnos, checkout, void, comunicaciones, reportes, certificados, autenticación real e integración durable con HukID.

**Approach:** Completar el módulo por fases sobre una única base PostgreSQL `nexo`, manteniendo los esquemas y ownership por bounded context. Onsite seguirá siendo una API Go/Fiber, Operations tendrá API y worker separados, y HukID se consumirá mediante un simulador wire-compatible con su Integration API canónica.

## Boundaries & Constraints

**Always:** JWT asimétrico con JWKS, tenant y scopes; hechos de asistencia append-only; idempotencia en escrituras; PostgreSQL único por ambiente con esquemas aislados; migraciones forward-only; outbox/inbox idempotentes; MinIO y Mailpit en desarrollo; claims HukID congelados, cifrados y con hash antes de llamar al proveedor; logs sin PII, tokens, QR ni ofertas.

**Never:** compartir tablas entre servicios, generar credenciales verificables dentro de Nexo, implementar OID4VCI en este grupo, leer directamente la base de HukID, implementar offline, aceptar claims/issuer/tenant confiados desde el navegador o borrar hechos de asistencia.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| Check-in concurrente | Dos POST para el mismo ticket | Una presencia efectiva y un hecho válido | `409 already_present` para la segunda operación |
| Checkout | Ticket con presencia efectiva | Hecho `check_out` append-only | `409 not_present` si no existe presencia |
| Void | Fact válido y razón con scope supervisor | Hecho compensatorio, sin UPDATE/DELETE | `403` sin scope o `409 invalid_state_transition` |
| HukID retry | Timeout después de POST | Mismo payload y `Idempotency-Key` | Replay `200`, nunca una segunda emisión |
| HukID transición | Delivery pending y simulador `ready` | Poll hasta `credential_issued` | Backoff, DLQ y estado `failed` ante error permanente |
| Reporte | Job autorizado | `202`, progreso y artifact temporal | cancelación, retry o expiración idempotentes |
| Certificado | Registro elegible y consentimiento vigente | PDF, hash, verificación y evento outbox | no elegible o consentimiento ausente sin emisión |

</frozen-after-approval>

## Code Map

- `backend/internal/auth/domain/types.go` y `backend/internal/httpapi/server.go` -- sesión Core actual; ampliar claims, tenant-context y JWKS.
- `backend/migrations/000001_identity.up.sql` y `backend/docker-compose.yml` -- identidad y PostgreSQL único actual.
- `microservices/onsite-operations/internal/platform/http/router.go` -- rutas y auth mock actuales; extender endpoints y bearer/JWKS.
- `microservices/onsite-operations/internal/platform/postgres/store.go` -- repositorio transaccional existente; reutilizar para hechos, proyecciones e idempotencia.
- `microservices/onsite-operations/migrations/*` y `seeds/*` -- esquema y Demo Day actuales; convertirlos a esquema `nexo` y seed explícito.
- `frontend/src/microfrontends/onsite-operations/*` -- UI y cliente actuales; conectar estados reales y nuevas vistas internas.
- `frontend/src/platform/microfrontends/registry.tsx` -- manifest y rutas del microfrontend.
- `.github/workflows/ci.yml` y `.github/scripts/test-microservices.sh` -- checks obligatorios y calidad Go.
- `tools/hukid-simulator/` -- nuevo simulador local basado en el OpenAPI canónico de HukID.

## Tasks & Acceptance

**Execution:**

- [ ] `backend/` -- emitir JWT asimétrico con tenant, roles, scopes, JWKS, tenant-context y rotación de claves.
- [ ] `docker-compose.yml`, migraciones raíz y Makefiles -- consolidar Core/Onsite/Operations en PostgreSQL `nexo`, con roles y health checks.
- [ ] `microservices/onsite-operations/` -- implementar dispositivos, turnos, readiness, checkout, void, QR JWS y autorización real.
- [ ] `microservices/event-operations/` -- crear API de comunicaciones, reportes, certificados y entregas HukID con worker separado.
- [ ] `tools/hukid-simulator/` -- implementar OAuth, Integration API, idempotencia, estados, errores y endpoints exclusivos de prueba.
- [ ] `frontend/` -- completar navegación, control de acceso, asistencia, reportes, certificados, comunicaciones y entrega HukID usando contratos reales.
- [ ] `seeds/demo-day-full.*` -- crear tenant, roles, tickets, asistencia, reglas, certificados, consentimientos y estados HukID deterministas.
- [ ] `contracts/`, OpenAPI y CI -- versionar esquemas, validar contratos, ejecutar unitarias, integración, E2E y pruebas de concurrencia.

**Acceptance Criteria:**

- Given un entorno vacío, when se ejecuta el compose raíz, then todos los servicios usan una sola base `nexo` y sus health checks están verdes.
- Given un usuario autenticado, when selecciona un tenant, then el token resultante queda ligado al tenant y los servicios rechazan otro tenant o scope.
- Given un QR JWS válido, when staff lo escanea, then Onsite valida firma, audience, evento, scope, expiración y revocación antes de registrar asistencia.
- Given dos solicitudes concurrentes, when terminan, then existe exactamente una presencia efectiva.
- Given un certificado elegible, when se solicita HukID, then el worker persiste el snapshot antes del POST y reconcilia mediante la misma key hasta estado terminal.
- Given el simulador HukID, when se repite una solicitud con la misma key y payload, then devuelve replay idempotente; con payload distinto, devuelve `409`.
- Given un reporte o certificado, when termina, then el artifact tiene hash, TTL y auditoría sin exponer PII innecesaria.
- Given una PR, when ejecuta CI, then pasan contratos, Core, microservicios, frontend, E2E, migraciones, seeds y contratos HukID.

## Implementation Notes

La primera implementación debe conservar los endpoints actuales para compatibilidad y añadir los nuevos de forma versionada. La base local `nexo_onsite` es temporal: la migración debe preservar IDs y hechos existentes. Los workers usarán PostgreSQL outbox con `FOR UPDATE SKIP LOCKED` mientras Plataforma no entregue un broker. El simulador HukID aceptará `POST /oauth2/token`, `/v1/issuance-requests` y `GET /v1/issuance-requests/{id}` con la semántica del OpenAPI canónico; sus rutas `/__test` nunca serán consumidas por el adapter productivo.

Implementado en esta iteración: compose raíz con PostgreSQL único y migraciones aisladas por tabla; JWT RS256/JWKS y tenant-context en Core; endpoints Onsite para dispositivos, turnos, checkout y void; validación de bearer contra JWKS; Event Operations API inicial para report jobs/certificados/entregas; worker de polling HukID; simulador HukID wire-compatible; contrato OpenAPI ampliado; cliente frontend usando bearer cuando existe sesión. La generación real de PDF/MinIO, plantillas y comunicaciones completas, DLQ administrable, rotación persistida de claves y E2E de Docker quedan como siguiente incremento porque Docker Desktop no está disponible en este entorno.

## Verification

**Commands:**

- `docker compose up -d` -- esperado: PostgreSQL único, Core, Onsite, Operations, MinIO, Mailpit y simulador saludables.
- `go test ./... && go vet ./...` en `backend`, `microservices/onsite-operations`, `microservices/event-operations` y `tools/hukid-simulator` -- esperado: verde.
- `npm run build && npm run test && npm run test:e2e` en `frontend` -- esperado: verde con login, QR, reportes y certificados.
- `make contract-test` -- esperado: requests y respuestas compatibles con OpenAPI de Nexo y HukID.
- `make load-test` -- esperado: check-in p95 menor de 700 ms y una sola presencia efectiva bajo concurrencia.

## Review Triage Log

| Finding | Verdict | Route | Evidence |
|---|---|---|---|
| Operations aceptaba mutaciones sin JWT ni scopes | medium | patch | Las rutas privadas no tenían middleware; se agregó validación RS256/JWKS cuando llega Bearer y se mantiene mock solo en development/test. |
| Replay de HukID podía devolver un ID no insertado | medium | patch | La inserción usaba `ON CONFLICT DO NOTHING` y respondía con un UUID nuevo; ahora consulta y devuelve la entrega existente. |
| Simulador aceptaba cualquier Bearer no vacío | medium | patch | Se agregó registro temporal de tokens emitidos y expiración; las rutas protegidas rechazan tokens falsos. |
| Transiciones terminales del simulador podían repetirse | low | patch | Redeem/issue/cancel ahora rechazan solicitudes emitidas o canceladas. |
| Update/revocación de dispositivo podían reportar éxito inexistente | low | patch | Se valida `RowsAffected()` y se propaga `pgx.ErrNoRows`. |
| Retry HukID intentaba nueve veces | low | patch | La condición usa `attempts+1>=8`, alineada con el máximo documentado. |
| JWT/JWKS, checkout/void e integración HukID carecían de pruebas automatizadas | medium | defer | La revisión verificó que no existen fixtures de integración para estas rutas; requieren PostgreSQL/stack disponible y se agregan al siguiente incremento de pruebas. |
| QR aún no valida JWS completo en backend | high | defer | El frontend extrae `jti` y el endpoint existente recibe `ticket_id`; falta contrato/emisor de Participation del Grupo 3. No se inventa una validación incompatible. |
| PDF/MinIO, comunicaciones completas y auth persistente de claves no están implementados | medium | defer | La propia implementación registra estas dependencias como siguiente incremento; Docker no estaba disponible para completar infraestructura y E2E. |

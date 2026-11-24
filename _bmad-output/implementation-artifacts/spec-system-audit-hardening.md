---
title: 'Corrección integral de hallazgos de auditoría de Nexo'
type: 'feature'
created: '2026-11-24'
status: 'in-review'
route: 'dispatch'
review_loop_iteration: 0
baseline_commit: 'eb8e31ab9bcfb60716b05e3acd029541c31a3f81'
context:
  - 'docs/engineering/POSTGRESQL-CONVENTIONS.md'
  - 'docs/engineering/MODULE-INTEGRATION.md'
  - 'CONTRIBUTING.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** La plataforma compila, pero la auditoría detectó fallos concretos de autorización, aislamiento tenant, arranque limpio, despliegue independiente, firma de tickets y conexión Participation–HukID–Onsite. Estos fallos permiten privilegios excesivos e impiden certificar el flujo completo.

**Approach:** Corregir cada hallazgo con una regla ejecutable, contrato versionado, migración reversible y prueba automatizada. El resultado arrancará desde una base vacía, operará con membresías reales y completará registro, credencial y check-in sin mocks de negocio.

## Boundaries & Constraints

**Always:** Una sola base `nexo`; ownership por schemas; comunicación por HTTP/eventos; JWT RS256 de Core; tickets ES256; producción falla cerrada; mocks sólo con `NEXO_TEST_MODE=true`; OpenAPI actualizado; frontend ES/EN.

**Never:** No compartir tablas entre servicios; no copiar nodo ni wallet HukID; no aceptar tenant, roles, claims o elegibilidad decididos por el navegador; no ejecutar seeds en despliegues selectivos; no hacer commit ni push.

## I/O & Edge-Case Matrix

| Caso | Entrada/estado | Resultado obligatorio |
|---|---|---|
| Login de `member` | Membresía limitada | Token tenant sólo con scopes de `member` |
| Sesión revocada | JWT firmado, `sid` inactivo | `401 session_revoked` antes de escribir |
| Arranque vacío | PostgreSQL sin schemas | Todos los schemas, roles, seeds y APIs quedan listos |
| Emisión válida | Ticket confirmado | Operations obtiene datos canónicos de Participation y emite una vez |
| Emisión manipulada | Ticket inválido/ajeno | `403/404/422`; no crea entrega |
| OID4VP válido | `completed/pass` | Onsite registra un solo check-in |
| OID4VP inválido | `pending/fail/expired` o contexto distinto | No registra asistencia |
| Concurrencia | Misma key/ticket | Una operación efectiva y replay consistente |

</frozen-after-approval>

## Correcciones trazadas a la auditoría

### A. Core, autenticación y autorización

1. Eliminar el `owner` y los scopes fijos de `createSession`. El token posterior al login sólo acreditará `sub`, `sid` y `auth:tenant-context`.
2. `POST /v1/auth/tenant-context` leerá `identity.memberships.role` y usará una matriz única para `super_admin`, `admin`, `manager`, `operator` y `member`. Sólo `super_admin` recibirá `tenant:delete`.
3. Crear `POST /v1/internal/auth/session-introspection`, protegido por `NEXO_INTERNAL_SERVICE_TOKEN`, que valide `sid`, `sub`, tenant y estado.
4. Onsite y Operations consultarán introspección con timeout y cache máximo de 30 segundos. Sesión revocada devuelve `401`; fallo sin cache válida devuelve `503`, nunca autoriza.
5. `X-Mock-*` y ausencia de bearer sólo funcionarán con `APP_ENV=test` y `NEXO_TEST_MODE=true`; desarrollo normal exigirá JWT.

### B. CORS y runtime

1. Onsite, Operations y Participation permitirán `Authorization`, `Content-Type`, `Idempotency-Key` y `X-Correlation-Id` sólo para `CORS_ALLOWED_ORIGINS`; Participation eliminará `*`.
2. Los contenedores Go ejecutarán usuario no root y Core, Onsite, Operations, Participation y simulador tendrán readiness healthcheck.
3. En producción, cada servicio rechazará defaults locales de claves, tokens, contraseñas y URLs inseguras.

### C. PostgreSQL, tenant y migraciones

1. Añadir `tenant_id` a forms, registrations y tickets de Participation; usar claves e índices compuestos. Backfill del seed con IncubaUNAS.
2. Añadir RLS tenant-scoped a Participation, Onsite, Operations, Certificates e Integration. Cada transacción autenticada establecerá `SET LOCAL app.tenant_id`; workers procesarán un tenant explícito por fila.
3. Crear roles migradores por contexto; runtime conservará sólo privilegios sobre su schema y `NOBYPASSRLS`.
4. Mover metadata a `platform_meta.{core,onsite,participation,operations}_schema_migrations`.
5. Corregir los `down` 000002/000003 de Operations para restaurar constraints, estados y nulabilidad; probar `up → down → up`.

### D. Participation y ticket seguro

1. Sustituir `development-signature` por JWS ES256 con `kid`; publicar `GET /.well-known/jwks.json` y conservar claves anteriores durante rotación.
2. Claims: `jti`, `iss`, `aud=onsite`, `tenant_id`, `event_id`, `registration_id`, `scope`, `iat`, `exp`, `version`; sin PII.
3. Crear `GET /participation/internal/v1/tickets/{ticketId}/credential-eligibility`, protegido por token de servicio. Devuelve referencias canónicas, estado, expiración y `evidence_digest`.
4. Al confirmar registro, insertar outbox `participation.ticket.confirmed.v1` en la misma transacción. El dispatcher solicitará emisión a Operations con key estable.

### E. Event Operations y emisión HukID

1. `POST /operations/v1/participation-credentials` aceptará sólo `ticket_id`; Operations consultará elegibilidad a Participation y derivará tenant/evento/registro.
2. Rechazar ticket inexistente, revocado, expirado, no confirmado o de otro tenant.
3. Reservar idempotency key y request hash atómicamente antes de HukID: mismo payload reproduce; payload distinto devuelve `409 idempotency_key_reused`.
4. Procesar boletos con `certificate_id NULL`; probar OAuth cache, `401`, `409`, `422`, `429`, `503`, timeout, expiración y concurrencia.
5. Mantener claims cifrados y congelados; no registrar ofertas, tokens, PII ni payload completo.

### F. HukID OID4VP y Onsite

1. Extender simulador con POST/GET verification sessions, bearer token, TTL, `pending/completed/expired`, `pass/fail`, report digest e idempotencia.
2. Añadir `/__test/verification/{id}/pass`, `/fail`, `/expire` y reset, disponibles sólo en test/development.
3. Onsite reservará la key localmente antes de crear la sesión remota; replay no creará una sesión huérfana.
4. Vincular sesión a tenant, ticket, evento y scope. Tras `pass`, revalidar proyección activa, expiración, revocación y asignación antes del check-in transaccional.
5. `pending/fail/expired`, sesión ajena o ticket distinto no crean asistencia. Polls concurrentes producen una sola presencia.
6. Frontend mostrará QR/launch URI y estados pendiente, aprobado, rechazado y expirado con design system e i18n; check-in directo sólo existirá en test.

### G. Arranque, despliegue y CI

1. `dev-up.sh` ejecutará `migrate-participation`, `seed-participation`, bootstrap de roles y levantará `participation-api`.
2. Agregar `deploy-service.sh participation` y `make deploy-participation`; migrará y recreará sólo Participation, sin seeds ni reiniciar otros servicios.
3. Agregar smoke de base vacía y prueba de despliegue selectivo comprobando que los IDs de contenedores ajenos no cambian.
4. E2E real: login, tenant, evento, registro Participation, ticket firmado, emisión HukID, presentación OID4VP, check-in y dashboard.
5. CI validará OpenAPI semántico, migraciones, `go test -race`, frontend, E2E completo y cobertura mínima de paquetes críticos.

## Code Map

- `backend/internal/httpapi/server.go`, `backend/internal/platform/postgres/` -- claims, membresías e introspección.
- `microservices/participation/internal/httpapi/server.go`, `migrations/` -- tenant, JWS/JWKS, elegibilidad y outbox.
- `microservices/event-operations/internal/httpapi/server.go`, `internal/worker/hukid.go`, `migrations/` -- emisión e idempotencia.
- `microservices/onsite-operations/internal/platform/http/router.go`, `internal/hukid/`, `migrations/` -- OID4VP y check-in.
- `tools/hukid-simulator/cmd/api/` -- simulación issuance/verificación.
- `docker-compose*.yml`, `scripts/`, `Makefile`, `.github/`, `frontend/e2e/` -- operación y gates.

## Tasks & Acceptance

**Execution:**
- [ ] Implementar A/B y pruebas negativas de rol, revocación, mock y CORS.
- [ ] Implementar C con migraciones, permisos y reversibilidad.
- [ ] Implementar D/E con contract tests e idempotencia concurrente.
- [ ] Implementar F con simulador, backend, frontend y pruebas de estados.
- [ ] Implementar G y ejecutar stack desde base limpia.

**Acceptance Criteria:**
- Given cualquier rol, when Core emite contexto tenant, then sólo contiene scopes de su matriz.
- Given token revocado o auth desconocida, when llega una escritura, then no modifica datos.
- Given dos tenants, when operan datos equivalentes, then ninguna API/worker cruza filas.
- Given ticket confirmado, when OID4VP termina en `pass`, then existe exactamente un check-in y el dashboard cambia.
- Given base vacía, when corre bootstrap, then todas las APIs quedan healthy y el E2E completo pasa.

## Implementation Notes

- Implementado: login sin scopes administrativos por defecto; los scopes se calculan al seleccionar tenant.
- Implementado: Participation se incluye en `dev-up`, tiene despliegue selectivo y los contenedores Go nuevos ejecutan como no-root.
- Implementado: mocks restringidos a `APP_ENV=test` + `NEXO_TEST_MODE=true`, headers CORS autenticados y CORS de Participation sin wildcard.
- Implementado: simulador HukID con sesiones OID4VP, estados `pending/completed/expired`, resultados `pass/fail`, TTL, token de operador de desarrollo y digest reproducible.
- Implementado: validación local de TTL y existencia tenant-scoped antes de consultar una sesión HukID; frontend acepta `launchUri` y `launch_uri`.
- Pendiente: introspección de sesión entre Core y consumidores, tenant/RLS completo de Participation, JWS ES256/JWKS real, elegibilidad servidor-a-servidor, reserva atómica antes de crear sesión remota, pruebas E2E completas y migraciones `platform_meta`. No se marcan como terminadas porque requieren cambios de contrato, persistencia y pruebas de integración aún no presentes.

## Spec Change Log

## Review Triage Log

## Design Notes

La revocación usa introspección HTTP para respetar ownership de `identity`. El cache corto tolera una caída momentánea, pero nunca autoriza una sesión desconocida.

El resultado OID4VP no entrega claims a Nexo. La sesión queda vinculada a la política del tenant y al contexto local: HukID determina autenticidad y Nexo revalida elegibilidad operacional.

## Verification

**Commands:**
- `docker compose config --quiet`
- `bash .github/scripts/test-microservices.sh`
- `go test -race ./... && go vet ./...` en cada servicio Go.
- `cd frontend && npm run tokens:validate && npm run build && npm run test && npm run test:e2e`
- `bash .github/scripts/test-clean-stack.sh`

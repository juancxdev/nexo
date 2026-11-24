# Equipo 4 · Aportes sobre `develop`

Rama: `feature/onsite-aportes-sobre-develop`, creada desde `origin/develop` (`d51bb2b`).

**Criterio:** la base es el código de Juan. Cuando ambos habíamos hecho lo mismo, se dejó lo suyo. Aquí solo entra lo que faltaba y los errores encontrados al probar. Se respetan sus decisiones:

- HukID emite la credencial y Nexo no produce PDF ni plantilla.
- La verificación pública no muestra el nombre del participante.
- Cada servicio es dueño de su schema.

## 1. Arreglos en Onsite (`ac8e5ee`)

| Problema en `develop` | Qué pasaba | Arreglo |
|---|---|---|
| `problem()` devolvía `nil` | Un operador **sin asignación** recibía 403, pero el handler seguía y **registraba la entrada igual**. | `problem()` devuelve `errResponded` y un `ErrorHandler` lo silencia, así la petición se corta. |
| Auditoría de la anulación | `jsonb_build_object('reason',$6)` sin tipo: **toda anulación fallaba** (`SQLSTATE 42P18`). | `$6::text`. |
| Doble anulación | Dos supervisores podían anular el mismo registro. | Índice único parcial y verificación previa (409 `already_voided`), con lock por ticket. |
| Anular una salida | Borraba la presencia siempre. | La presencia se recalcula desde la historia (`EffectivePresence`): anular una salida devuelve a la persona a "presente". |
| Motivo | Bastaba un espacio o una "x". | Mínimo 5 caracteres (422 `reason_required`) para anular y para revocar. |
| Revocar dispositivo | No guardaba el motivo y dejaba turnos abiertos. | `revoked_reason` y cierre de los turnos abiertos en la misma transacción. |
| Rol del bearer | El rol normalizado (owner/admin → manager) no llegaba a los handlers. | Se guarda en `Locals`. |

Migración `000008`. Pruebas: `tests/aportes_test.go`.

## 2. QR firmado, rechazos, provisional y sesiones (`f95e2cb`)

**Participation.** El QR pasa de `HS256` con `"development-signature"` (firma falsa) a un **JWS EdDSA (Ed25519)**:

- La clave se toma de `PARTICIPATION_TICKET_SIGNING_SEED` (32 bytes en base64). Si no está definida, se usa una clave de desarrollo conocida.
- `scope` pasa a ser la zona (`main`). El slug del evento va aparte, en `event_slug`.
- `GET /participation/public/v1/ticket-keys` publica la clave pública (JWKS).

**Onsite**, sobre el formato que ya emite Participation:

| Endpoint | Para qué |
|---|---|
| `POST /onsite/v1/scans` | Verifica algoritmo (solo EdDSA), `kid`, firma, `aud`, vigencia, evento (acepta el ID de Participation vía `events.participation_event_id`), zona y revocaciones. |
| `POST /onsite/v1/scans/provisional` | Supervisión acepta un ticket recién emitido que aún no está proyectado. Solo si la lista de revocaciones está fresca. Queda auditado. |
| `GET /onsite/v1/scan-stats` | Rechazos de 24 h por motivo, sin datos personales. |
| `GET /onsite/v1/sessions`, `POST /onsite/v1/sessions/:code/entries` | Sesiones con horario, acceso restringido y aforo (con lock), sin entrar dos veces. No tocan la asistencia principal. |
| `GET /onsite/v1/shifts` | Turnos del evento, para la pantalla de dispositivos. |

Migración `000009` y seed `seeds/demo-day-qr.sql`, que ya aplica `docker-compose.init.yml`. El seed contiene:

- la clave pública de desarrollo;
- el alias `demo-day-2026` → `event-demo`;
- sesiones de prueba;
- la asignación de supervisor demo.

**Probado de punta a punta.** Me registré en Participation y el QR resultante fue verificado por Onsite. Supervisión lo aceptó como provisional y el check-in quedó registrado.

## 3. Reportes reales y certificados (`9174820`)

**Contratos de lectura.** Event Operations **no** lee tablas de Onsite ni de Participation. Solo lee vistas versionadas `security_barrier`, creadas por las migraciones de cada dueño:

- `onsite_contract.events_v1`, `attendance_v1`, `presence_v1` y `scan_rejections_v1` (titular enmascarado);
- `participation_contract.registrations_v1` (solo confirmados, sin correo ni tokens).

Se comprobó que `operations_app` puede leer las vistas y recibe `permission denied` en `onsite` y `participation`. Las vistas se conceden en `scripts/bootstrap-runtime-roles.sql`.

**Reportes (OO-020/021).**

- Antes, el worker marcaba `succeeded` **sin generar ningún archivo**.
- Ahora genera CSV (con BOM UTF-8 y fórmulas neutralizadas) o JSON con datos reales.
- Tipos: `attendance`, `presence`, `rejections` y `summary`.
- Usa `SKIP LOCKED` y lease, y guarda el sha256 del archivo.
- Endpoints nuevos: listado, descarga (409 si no está listo, 410 si se purgó), reintento y purga. Además hay purga automática al vencer.
- Migración `000003`.

**Certificados (OO-024/025/026).**

- Elegibilidad `attendance@1.0.0`: el registro debe estar confirmado en Participation y tener una entrada **vigente** (no anulada) en Onsite.
- El nombre sale de Participation; el `display_name` que envía el cliente se ignora.
- Un solo certificado vigente por participante (409 `already_issued`).
- Revocación con motivo, que cancela las entregas HukID abiertas.
- Reemisión enlazada mediante `replaces_id` y `replaced_by`.
- Verificación pública: `?format=html` (o `Accept: text/html`) devuelve una página legible sin el nombre del titular. El JSON se mantiene igual.
- Migración `000004`.

## 4. Frontend (`4ec5b5e`)

Las pantallas están en `OnsitePanels.tsx` y usan los componentes del design system. Los textos están en `core/onsite-i18n.ts`, con el mismo patrón que `events-i18n`.

- **Control de acceso.** El QR firmado se verifica en el backend; antes se decodificaba en el navegador sin comprobar la firma. Muestra el motivo de rechazo legible y la opción de aceptar como provisional.
- **Asistencia.** Registrar salida y anular con motivo.
- **Pestañas nuevas:** Sesiones, Dispositivos y turnos, Certificados, y Reportes con los rechazos de QR.
- **CORS de Event Operations.** Ahora admite `X-Mock-Role`. Sin eso, en modo demo fallaba el preflight y el botón "Generar reporte" no funcionaba.

## Cómo probar

```bash
# Instalación limpia (crea la base y aplica migraciones, seeds y roles)
./scripts/dev-up.sh

# Pruebas con base de datos
cd microservices/onsite-operations && APP_ENV=test ONSITE_TEST_DATABASE_URL=postgres://nexo:nexo-local-only@localhost:5432/nexo?sslmode=disable go test ./...
cd microservices/event-operations  && APP_ENV=test OPERATIONS_TEST_DATABASE_URL=postgres://nexo:nexo-local-only@localhost:5432/nexo?sslmode=disable go test ./...
cd microservices/participation     && go test ./...
cd frontend && npx tsc -b && npx vitest run
```

Verificado en una base nueva: todas las migraciones y seeds se aplican en el orden de `docker-compose.init.yml` y todas las pruebas pasan (Go y 28 de vitest).

## Pendiente o a decidir con el equipo

- **Proyección de tickets Participation → Onsite.** No existe todavía. Por eso un ticket recién emitido aparece como `ticket_not_materialized` y lo resuelve supervisión (OO-017). Cuando exista, entrará directo.
- **Revocaciones.** `ticket_revocations` y `revocation_state` se llenan hoy por seed. Falta el sincronizador desde Participation.
- **Clave de producción.** Hay que definir `PARTICIPATION_TICKET_SIGNING_SEED` y registrar su clave pública en `onsite.ticket_keys`.
- **HukID.** Revocar un certificado cancela las entregas pendientes, pero no revoca una credencial ya emitida en HukID. Eso requiere el endpoint de revocación del emisor.

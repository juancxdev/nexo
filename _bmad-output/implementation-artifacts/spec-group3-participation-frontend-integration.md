---
title: 'Integración del frontend de Participation del Grupo 3'
type: 'feature'
created: '2026-11-19'
status: 'in-progress'
route: 'oneshot'
review_loop_iteration: 0
context:
  - '/Users/juancx/Documents/CX-Developments/SELF/nexo/docs/engineering/GROUP3-INTEGRATION-AUDIT.md'
  - '/Users/juancx/Documents/CX-Developments/SELF/nexo/docs/engineering/MODULE-INTEGRATION.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** El Grupo 3 entrega `apps/participation-web` como una aplicación Vite independiente, con API fija en `localhost:8000`, navegación propia y estilos que no pertenecen al shell de Nexo. No puede convivir con la autenticación, i18n, rutas ni sistema de diseño actuales.

**Approach:** Integrar el flujo público de inscripción y consulta de ticket como un microfrontend React dentro de `frontend`, reutilizando tokens y componentes de Nexo. La API será configurable mediante `VITE_PARTICIPATION_API_URL` y el cliente respetará el contrato `/participation/public/v1/...`; no se copiarán el Compose ni la base PostgreSQL aislada del repositorio externo.

## Boundaries & Constraints

**Always:** usar `ModuleContext` y el registro de microfrontends; mantener español e inglés; labels asociados, estados de carga/error y layout responsive; enviar `Idempotency-Key` en comandos; mantener la API de Participation como autoridad de registros, cupos y tickets.

**Never:** copiar la aplicación monolítica completa, introducir otra base o servicio en Compose, usar `localhost:8000` fijo, duplicar autenticación o modificar el dominio de Event Configuration/Onsite.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Carga del formulario | `site_slug`, `event_slug` configurables | Renderiza versión y consentimiento activos | Muestra estado de API no disponible sin romper el shell |
| Registro exitoso | Datos válidos + `Idempotency-Key` | Muestra receipt y permite consultar ticket | Conserva datos y muestra respuesta RFC 9457 |
| Registro repetido | Misma key y payload | Reproduce el receipt sin duplicar | Key reutilizada con otro payload muestra conflicto |
| Ticket | Receipt/access token válido | Muestra estado y QR sin PII | Oculta ticket revocado, expirado o no autorizado |

</frozen-after-approval>

## Code Map

- `frontend/src/platform/microfrontends/contracts.ts` -- contrato de contexto y manifest que debe implementar el módulo.
- `frontend/src/platform/microfrontends/registry.tsx` -- registro, rutas y capacidades visibles en el shell.
- `frontend/src/shell/AppShell.tsx` -- montaje de módulos autenticados y rutas públicas.
- `frontend/src/design-system/atoms.tsx`, `frontend/src/design-system/molecules.tsx` -- componentes y patrones visuales reutilizables.
- `/tmp/nexo-group3-audit.mUvhj0/apps/participation-web/src/App.tsx` -- referencia funcional del Grupo 3; reutilizar flujos, no su estructura monolítica ni estilos.
- `/tmp/nexo-group3-audit.mUvhj0/services/participation/contracts/openapi.yaml` -- contrato externo de Participation que consume el frontend.
- `docs/engineering/GROUP3-INTEGRATION-AUDIT.md` -- límites de integración, ownership y pendientes deliberados.

## Tasks & Acceptance

**Execution:**
- [ ] `frontend/src/microfrontends/participation/api.ts` -- crear cliente tipado para formulario, registro, receipt y ticket con base configurable e idempotencia.
- [ ] `frontend/src/microfrontends/participation/ParticipationModule.tsx` -- implementar el flujo público responsive usando el design system y estados accesibles.
- [ ] `frontend/src/microfrontends/participation/participation.css` -- añadir únicamente estilos de composición basados en tokens existentes.
- [ ] `frontend/src/shell/AppShell.tsx` y `frontend/src/platform/microfrontends/registry.tsx` -- montar la ruta pública `/participation` sin exigir sesión y registrar el microfrontend.
- [ ] `frontend/src/core/i18n.ts` -- añadir textos ES/EN del flujo de participación.
- [ ] `frontend/src/microfrontends/participation/*.test.tsx` -- cubrir carga, registro exitoso, error RFC 9457 y cambio de idioma.

**Acceptance Criteria:**
- Given una visita sin sesión a `/participation`, when se carga la página, then se muestra el formulario público sin redirigir al login.
- Given una API configurada, when el participante envía datos válidos, then el cliente envía `Idempotency-Key` y muestra el receipt retornado.
- Given una respuesta de error del API, when termina la solicitud, then se muestra un mensaje accesible y se conserva el formulario.
- Given el cambio de idioma del shell, when cambia ES/EN, then todos los textos del microfrontend se actualizan.
- Given una vista de 375px, when se renderiza el flujo, then no aparece scroll horizontal ni controles menores a 44px.

### Review Findings

- [ ] [Review][Patch] El frontend usa `http://localhost:8000` como fallback para `VITE_PARTICIPATION_API_URL` y falla fuera del entorno local [frontend/src/shell/AppShell.tsx:30].
- [ ] [Review][Patch] La ruta pública sólo reconoce `/participation` exacto y no variantes como `/participation/` o rutas públicas versionadas [frontend/src/shell/AppShell.tsx:20].
- [ ] [Review][Patch] El montaje público está acoplado a una condición específica dentro de `AppShell` en vez de resolverse genéricamente desde el manifest [frontend/src/shell/AppShell.tsx:30].
- [ ] [Review][Patch] `publicOnly` excluye el módulo de `visibleModules` y obliga a un bypass manual; faltan pruebas que protejan la separación entre navegación autenticada y resolución pública [frontend/src/platform/microfrontends/contracts.ts:6].
- [ ] [Review][Patch] No existe prueba E2E que abra `/participation` sin sesión, verifique la API configurada y complete una inscripción [frontend/e2e/].
- [ ] [Review][Defer] No existe aún backend Participation, schema/migraciones `participation`, seeds, OpenAPI ejecutable, Compose ni integración CI con la base central; el frontend no puede validarse punta a punta [microservices/].
- [ ] [Review][Defer] Event Operations no tiene pruebas propias y `microservices-quality` sólo ejecuta `go test ./...` sin detectar la ausencia de tests; HukID queda sin cobertura de worker [microservices/event-operations/].
- [ ] [Review][Defer] El adaptador HukID no tiene pruebas para `401`, `409`, `422`, `429`, `503`, expiración, concurrencia ni replay idempotente [microservices/event-operations/internal/worker/hukid.go].
- [ ] [Review][Defer] `createDelivery` permite omitir `Idempotency-Key` y genera una clave implícita; además, dos solicitudes concurrentes del mismo certificado pueden terminar en `503` por la restricción única en lugar de una respuesta idempotente [microservices/event-operations/internal/httpapi/server.go:319].
- [ ] [Review][Defer] El worker no reclama filas con `FOR UPDATE SKIP LOCKED` y no clasifica explícitamente las respuestas HTTP del polling, por lo que varias instancias pueden procesar una entrega simultáneamente y tratar `401/409/503` como respuestas genéricas [microservices/event-operations/internal/worker/hukid.go:69].
- [ ] [Review][Defer] El simulador HukID sólo inyecta fallos en creación; no puede forzar fallos del token, polling, expiración ni respuestas concurrentes, por lo que no permite probar el contrato completo [tools/hukid-simulator/cmd/api/main.go:47].

#### Hallazgos rechazados

- Rechazado: el microfrontend no aparece en el diff. Los archivos están presentes en el working tree y el build los compila; el hallazgo provenía de que los archivos nuevos no estaban incluidos en `git diff` al no estar trackeados.
- Rechazado: textos fijos del manifest como defecto de internacionalización. `publicOnly` evita que el manifest se muestre en el sidebar y no existe consumo visible de esos textos en esta integración.

## Implementation Notes

Se integra primero el frontend porque el repositorio actual todavía no contiene una Participation API ejecutable. La adaptación del backend, migraciones al schema `participation` y seeds queda fuera de este incremento y deberá seguir el contrato del Grupo 3 antes de activar el endpoint real.

Se creó un microfrontend público bajo `/participation` con parámetros opcionales `?site=` y `?event=`, cliente configurable, formulario dinámico basado en `schema.properties`, clave de idempotencia estable durante el intento, receipt, estados de ticket y QR descargable generado desde el JWS. Se mantuvieron las bases de datos, Compose y autenticación del repositorio externo fuera de Nexo.

La revisión encontró siete riesgos de integración; se corrigieron el formulario estático, `answers` vacío, QR ausente, tickets no válidos visibles, idempotencia inestable y ruta fija. Los mensajes de error del cliente conservan el idioma activo para errores sin detalle útil.

## Review Triage Log

- corregido -- el formulario ahora renderiza campos definidos por `schema.properties` y envía sus respuestas en `answers`.
- corregido -- el ticket activo se convierte en QR descargable; estados revocado, expirado y reemplazado no se presentan como utilizables.
- corregido -- la clave de idempotencia se conserva durante el intento de registro y se reusa ante re-render o reintento.
- corregido -- la ruta acepta `site` y `event` por query string, conservando valores de desarrollo como fallback.
- aceptado -- el backend de Participation no se incorpora en esta rama porque la solicitud actual integra el frontend; su migración al schema central queda pendiente del siguiente incremento.

## Verification

**Commands:**
- `cd frontend && npm test` -- esperado: todas las pruebas unitarias pasan.
- `cd frontend && npm run build` -- esperado: TypeScript y Vite compilan sin errores.
- `cd frontend && npm run tokens:validate` -- esperado: no se introducen tokens inválidos.

**Manual checks:**
- Abrir `/participation` sin sesión, cambiar ES/EN y verificar registro, error y responsive en 375px.

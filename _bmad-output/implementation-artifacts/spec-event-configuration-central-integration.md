---
title: 'Integrar Event Configuration al PostgreSQL y orquestador central de Nexo'
type: 'feature'
created: '2026-11-17'
status: 'done'
baseline_commit: '0d7c6f2862d6a19b30cb68785a69eda2719b8400'
route: 'dispatch'
review_loop_iteration: 0
context:
  - 'docs/engineering/POSTGRESQL-CONVENTIONS.md'
  - 'docs/engineering/MODULE-INTEGRATION.md'
  - 'microservices/event-configuration/docs/openapi.yaml'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** El código del Grupo 2 ya vive en `develop`, pero Event Configuration continúa siendo un entorno aislado: crea otra instancia PostgreSQL, mantiene dos árboles de migraciones, no participa en el compose raíz ni se valida en CI. Eso contradice la regla de una única base lógica `nexo` y permite que API, worker y microfrontend diverjan del entorno real.

**Approach:** Incorporar API, worker, migraciones y seed de Eventos al entorno central con el schema propietario `events` y roles limitados. Mantener los contenedores como unidades independientes: cada imagen podrá reconstruirse, desplegarse y reiniciarse por servicio sin reconstruir ni reiniciar Core, Onsite u Operations; PostgreSQL será la única dependencia compartida.

## Boundaries & Constraints

**Always:** una base PostgreSQL lógica `nexo`; propiedad exclusiva del schema `events`; `events_migrator` para estructura y `configuration_app` para API/worker sin superusuario ni bypass RLS; `db/nexo-migrations` como única fuente de migraciones; servicios independientes con health checks, `restart` y comandos de operación selectiva; pruebas Node/Nest dentro del check agregado `microservices-quality`; autenticación productiva compatible con JWT RS256/JWKS de Core y scopes `events:*`.

**Never:** crear otra base para Eventos en el stack normal; usar tablas de otros schemas; conceder credenciales administrativas a API/worker; eliminar o reescribir migraciones ya aplicables; reconstruir/reiniciar servicios no seleccionados para desplegar Eventos; aceptar `X-Mock-*` como contrato fuera de development/test.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Arranque limpio | Volumen PostgreSQL nuevo | Se crean roles, schema, metadata, migraciones y seed de Eventos antes de API/worker | El servicio no inicia si bootstrap/migración falla |
| Despliegue selectivo | Cambio sólo en Eventos | `docker compose up -d --build event-configuration-api event-configuration-worker` recrea únicamente esos contenedores | Los demás servicios mantienen su estado |
| Migración repetida | Metadata válida y versiones aplicadas | No altera estructura ni seed existente | Fallo explícito ante checksum/metadata dirty |
| Token Core | JWT RS256 con tenant y `events:*` | API valida firma/JWKS, issuer, audience, expiración y scope | 401/403 sin filtrar datos de otro tenant |
| CI de Eventos | PR modifica servicio Node | Ejecuta instalación bloqueada, build, lint, tests y contratos | Falla el check agregado sin ocultar servicios Go |

</frozen-after-approval>

## Code Map

- `docker-compose.yml` -- orquestador local de Core, Onsite, Operations y Eventos sobre el PostgreSQL compartido.
- `microservices/event-configuration/Dockerfile` -- imagen Node reutilizable para API, worker y jobs de bootstrap/migración.
- `microservices/event-configuration/scripts/bootstrap-shared.mjs` -- aprovisiona en forma idempotente los roles, schema y metadata centrales de Eventos.
- `microservices/event-configuration/scripts/migrate.mjs` -- consume `db/nexo-migrations` y metadata `platform_meta.events_*` con precondiciones de base/rol central.
- `microservices/event-configuration/db/nexo-migrations/` -- fuente oficial 0001–0008; el árbol `db/migrations/` obsoleto fue retirado.
- `microservices/event-configuration/apps/configuration-api/src/shared/auth/*` -- valida JWT RS256/JWKS de Core, claims de tenant y scopes de eventos; mock sólo en desarrollo/pruebas.
- `microservices/event-configuration/apps/configuration-api/tsconfig.json` -- el build incremental puede reutilizar metadata fuera de `dist` y dejar el artefacto incompleto; la metadata debe limpiarse junto al output.
- `backend/internal/httpapi/server.go` -- emite los scopes por rol; debe incluir `events:read`, `events:write` y `events:publish` donde corresponda.
- `.github/workflows/ci.yml` y `.github/scripts/test-microservices.sh` -- check agregado Go actual; extenderlo para validar el workspace pnpm sin romper el descubrimiento Go.
- `docs/engineering/POSTGRESQL-CONVENTIONS.md`, `docs/engineering/MODULE-INTEGRATION.md`, `README.md` y `microservices/event-configuration/README.md` -- reglas y operación deben reflejar el stack central y los despliegues selectivos.

## Tasks & Acceptance

**Execution:**

- [x] `docker-compose.yml` y `Makefile` -- añadir bootstrap, migración, seed, API y worker de Eventos; declarar health checks, dependencias sólo contra PostgreSQL/migración y objetivos selectivos `events-up`, `events-rebuild`, `events-logs`, `events-down`.
- [x] `microservices/event-configuration/scripts/` y `Dockerfile` -- crear un bootstrap central idempotente de roles/schema/metadata, exigir base `nexo` para el modo central y copiar sólo la fuente de migraciones oficial.
- [x] `microservices/event-configuration/db/migrations/` -- retirar el árbol duplicado; preservar intacto `db/nexo-migrations/` y añadir sólo migraciones forward-only si aparece una corrección real.
- [x] `microservices/event-configuration/compose.yaml`, `.env.example` y `README.md` -- retirar la ruta aislada como ejecución normal; si permanece como fixture de tests, nombrarla y documentarla explícitamente como efímera y no compartida.
- [x] `apps/configuration-api/src/shared/auth/*`, `backend/internal/httpapi/server.go` y pruebas asociadas -- validar JWKS RS256 de Core y emitir/autorizar los scopes de Eventos por tenant.
- [x] `apps/configuration-api/tsconfig.json`, `.github/workflows/ci.yml`, `.github/scripts/test-microservices.sh` y scripts pnpm -- hacer el build reproducible y ejecutar lint, build, pruebas unitarias y contratos del servicio Node dentro de `microservices-quality`, conservando la validación agregada de los servicios Go.
- [x] `docs/engineering/*`, `README.md` y pruebas -- documentar la topología y probar bootstrap/migraciones, health endpoints, autenticación negativa y que una actualización selectiva no toque servicios no seleccionados.

**Acceptance Criteria:**

- Given un volumen limpio, when se ejecuta el stack central, then Event Configuration API y worker usan `nexo/events`, sus migraciones se registran en `platform_meta.events_*` y ningún servicio crea `event_configuration`.
- Given un cambio en Eventos, when se ejecuta el objetivo selectivo, then sólo API/worker de Eventos se reconstruyen o reinician y Core, Onsite y Operations siguen disponibles.
- Given una migración repetida o una metadata inválida, when corre el job, then el resultado es idempotente o falla antes de iniciar la API.
- Given un token emitido por Core para el tenant activo, when llama a Eventos, then se valida por JWKS y se aplican los scopes `events:*`; un token/scope/tenant inválido no obtiene datos.
- Given una PR que modifica Eventos, when ejecuta CI, then `microservices-quality` valida tanto servicios Go como el workspace Node/Nest.

## Implementation Notes

El compose raíz seguirá siendo un orquestador de desarrollo, no un binario monolítico. Docker Compose conserva contenedores e imágenes independientes: los comandos selectivos serán la interfaz operativa local; en producción el mismo principio se traduce a imágenes/deployments independientes con configuración compartida de PostgreSQL. Un fallo de Core puede impedir nueva autenticación, pero no debe detener los procesos de Eventos, Onsite u Operations; cada servicio debe degradar sólo sus llamadas remotas y conservar su health de proceso/base cuando corresponda.

Implementado el 2026-11-17. Se añadió el bootstrap central de `events`, roles restringidos y metadata de migraciones bajo `platform_meta`; los ocho scripts canónicos se ejecutaron sobre `nexo`. El compose raíz ahora incluye API y worker de Eventos con health checks y no declara dependencia de ejecución con Core. La autenticación valida JWT RS256 mediante JWKS de Core y el Shell obtiene un token contextual de tenant.

## Spec Change Log

## Review Triage Log

- `frontend/src/shell/AppShell.tsx` — **medium / patch**: el microfrontend de Eventos usaba una URL vacía y habría llamado al origen del frontend; se fijó el fallback local `http://localhost:3000`.
- `DatabaseService` — **false**: la conexión ya define `search_path=events,public` y Kysely usa `.withSchema('events')`, por lo que las consultas no resuelven por accidente en `public`.
- `scripts/bootstrap-shared.mjs` — **medium / patch**: una cuenta de ejecución existente podía conservar membresías o privilegios ajenos; el bootstrap ahora los revoca, restaura sólo `CONNECT` y limita el rol fuera del schema `events`.
- `scripts/migrate.mjs` — **medium / patch**: tablas o secuencias futuras podían quedar sin permisos para la API; se añadieron privilegios por defecto del rol migrador además de los grants actuales.
- `docker-compose.yml` (worker) — **low / patch**: el worker podía arrancar antes del seed de desarrollo; ahora espera `seed-events` como la API.
- `backend/internal/httpapi/server.go` — **medium / patch**: un rol desconocido recibía `events:read`; el fallback vuelve a conceder sólo la capacidad base de Onsite y queda cubierto por prueba.
- `auth.guard.ts` — **medium / patch**: JWT correctamente firmado pero aún no válido (`nbf`) era aceptado; se valida `nbf` con tolerancia de reloj limitada.
- `auth.guard.ts` — **medium / patch**: una JWKS malformada podía escapar como error interno; el parseo y las claves inválidas ahora fallan cerradamente con `401`.
- `auth.guard.ts` — **medium / patch**: la recuperación de JWKS no tenía límite temporal; se agregó cancelación a los tres segundos y prueba de la señal de abortado.
- `ci.yml` — **medium / patch**: no existía una verificación automatizada del compose central; se añadió un smoke test con proyecto y volumen efímeros que prueba roles, migración idempotente y health checks.
- `auth.guard.spec.ts` — **medium / patch**: faltaba cobertura negativa de claims ambiguos o mal formados; la guardia ahora se prueba contra subject, tenant, roles y scopes inválidos.
- `microservices/event-configuration/Dockerfile` — **high / patch**: el fixture conservado invocaba `bootstrap-local.mjs` pero la imagen no lo copiaba; se restauró ese artefacto de fixture sin reincorporar una base aislada al stack normal.
- `backend/internal/httpapi/server_test.go` — **medium / patch**: los scopes de Eventos emitidos por `tenant-context` no estaban cubiertos por contrato; se valida el JWT Core real y sus tres scopes de owner.
- `frontend/src/core/session.test.ts` — **medium / patch**: la hidratación podía omitir el canje contextual sin señal de CI; la prueba exige `/me`, `/tenant-context` y la persistencia del nuevo token.
- `docker-compose.yml` y `.github/scripts/test-event-configuration-compose.sh` — **medium / patch**: la ruta de arranque central no tenía prueba limpia; el smoke test ejecutado crea un proyecto Docker aislado, verifica `configuration_app` sin privilegios y elimina sólo su volumen al terminar.

## Design Notes

La dependencia compartida se limita a PostgreSQL y a contratos explícitos. `depends_on` se usa para el bootstrap local, no como acoplamiento de disponibilidad de negocio: Eventos no dependerá de Core para arrancar. Las APIs se reconstruyen por nombre de servicio, por ejemplo:

```bash
docker compose up -d --build event-configuration-api event-configuration-worker
docker compose logs -f event-configuration-api event-configuration-worker
```

## Verification

**Commands:**

- `docker compose config` -- esperado: servicios y dependencias de Eventos válidos sobre el PostgreSQL raíz.
- `docker compose up -d postgres bootstrap-events migrate-events seed-events event-configuration-api event-configuration-worker` -- esperado: health checks verdes y una sola base `nexo`.
- `pnpm --dir microservices/event-configuration lint && pnpm --dir microservices/event-configuration build:api && pnpm --dir microservices/event-configuration test:api && pnpm --dir microservices/event-configuration test:event-schemas` -- esperado: calidad y contratos verdes.
- `bash .github/scripts/test-microservices.sh` -- esperado: validación Go y Node dentro del mismo check agregado.

**Resultado observado (2026-11-17):**

- `docker compose config` pasó.
- `docker compose up -d postgres bootstrap-events migrate-events seed-events event-configuration-api event-configuration-worker` dejó la API y worker saludables; `events` quedó propiedad de `events_migrator` y las cuentas de ejecución no tienen `SUPERUSER` ni `BYPASSRLS`.
- Una segunda ejecución de `docker compose run --rm migrate-events` fue idempotente y validó las migraciones existentes.
- Un usuario/tenant creado mediante Core obtuvo un token de `/v1/auth/tenant-context`; `GET /configuration/v1/events` con ese JWT devolvió `200`, mientras que un token inválido devolvió `401`.
- `docker compose up -d --build event-configuration-api event-configuration-worker` preservó el mismo identificador del contenedor `core-api`; sólo se actualizaron los servicios de Eventos.
- `pnpm install --frozen-lockfile`, build, tests, contratos, composición, worker, esquema, formato y admin build de Eventos pasaron; `npm run build && npm run test -- --run` de `frontend` y `go test ./... && go vet ./...` de `backend` también pasaron.
- `bash .github/scripts/test-microservices.sh` pasó, incluyendo servicios Go y el workspace Node/Nest de Eventos.

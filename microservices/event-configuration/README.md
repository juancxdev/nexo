# Event Configuration — Nexo

El Grupo 2 administra eventos, agendas, sedes, ponentes, formularios y publicación. El módulo contiene:

- API NestJS: `event-configuration-api` en el puerto `3000`.
- Worker de outbox: `event-configuration-worker`.
- Microfrontend React integrado en `frontend/src/microfrontends/events`.
- Schema PostgreSQL propietario: `events` dentro de la única base lógica `nexo`.

## Entorno integrado

Desde la raíz de Nexo, el stack central provisiona los roles `events_migrator` y `configuration_app`, migra `db/nexo-migrations/`, carga el seed de desarrollo y levanta API/worker:

```bash
make events-up
make events-rebuild   # sólo reconstruye y recrea API/worker de Eventos
make events-logs
make events-down
```

Los contenedores son independientes. Un cambio en Eventos no reconstruye Core, Onsite ni Operations. PostgreSQL es la dependencia compartida, pero Eventos sólo puede acceder a su schema `events`; no debe leer ni escribir schemas ajenos.

La API expone Swagger en `http://localhost:3000/configuration/v1/docs` durante desarrollo. Las migraciones oficiales viven exclusivamente en `db/nexo-migrations/`; no se editan después de integrarse.

## Seguridad y autenticación

En el stack central, la API valida JWT RS256 emitidos por Core mediante `CORE_JWKS_URL`. El tenant, roles y scopes se derivan del token; `X-Tenant-Id` sólo puede coincidir con el tenant del token y nunca lo reemplaza. Los headers `X-Mock-*` pertenecen únicamente a development/test.

## Fixture aislado

`compose.fixture.yaml` conserva un PostgreSQL efímero para pruebas locales aisladas del módulo. No es parte del stack normal y no debe usarse para integrar otros equipos ni contra la base compartida.

## Calidad

```bash
pnpm install --frozen-lockfile
pnpm lint
pnpm build:api
pnpm test:api
pnpm test:contract-consumer
pnpm test:composition
pnpm test:worker
pnpm test:event-schemas
pnpm format:check
pnpm build:admin
```

Consulta [OpenAPI](docs/openapi.yaml) y [la guía de integración externa](docs/EXTERNAL-INTEGRATION.md).

# Convenciones PostgreSQL de Nexo

## Regla objetivo

Nexo tendrá una única base lógica llamada `nexo`. Los límites de datos son schemas, no bases por microservicio:

```text
nexo
├── identity       # Core API
├── tenants        # Core API
├── onsite         # Onsite Operations
├── events         # Equipo de Eventos
├── cms            # Equipo CMS/Landing
└── platform_meta  # sólo metadata de migraciones
```

Un servicio es dueño de su schema, de sus migraciones y de su rol PostgreSQL. Ningún servicio hace joins ni escrituras sobre schemas de otro servicio.

## Operación central y contenedores independientes

El compose raíz opera una instancia PostgreSQL `nexo` y contenedores separados por servicio. Compartir PostgreSQL no convierte el sistema en monolítico: cada API y worker se construye, reinicia y despliega por nombre de servicio. En desarrollo, reconstruir Eventos no debe recrear Core, Onsite ni Operations:

```bash
docker compose up -d --build event-configuration-api event-configuration-worker
```

Cada servicio depende de PostgreSQL y de su job de migración para el primer arranque; no debe depender de otro API para mantenerse saludable. Los compose con bases aisladas sólo pueden existir como fixtures explícitos de pruebas y nunca son la ruta normal del proyecto.

## Migraciones por servicio

Las migraciones viven con el servicio y empiezan en `000001`; la metadata no puede compartir la tabla default `schema_migrations`.

Cada servicio usa una tabla exclusiva dentro de `platform_meta`:

```text
platform_meta.identity_schema_migrations
platform_meta.onsite_schema_migrations
platform_meta.events_schema_migrations
```

Con `golang-migrate`, la URL debe fijar el schema y una tabla de control calificada. Ejemplo para Eventos:

```text
postgres://events_migrator:<secret>@postgres:5432/nexo?sslmode=disable&search_path=events&x-migrations-table=%22platform_meta%22.%22events_schema_migrations%22&x-migrations-table-quoted=true
```

`golang-migrate` soporta `search_path`, `x-migrations-table` y `x-migrations-table-quoted`; la tabla separada evita que servicios con archivos `000001` colisionen. La referencia oficial está en la [documentación de golang-migrate](https://github.com/golang-migrate/migrate/blob/master/database/postgres/README.md).

## Roles mínimos

| Rol | Permisos |
| --- | --- |
| `<service>_runtime` | `USAGE` en su schema y CRUD sólo sobre sus tablas/secuencias. |
| `<service>_migrator` | `USAGE, CREATE` en su schema y acceso a su tabla dentro de `platform_meta`. |
| `platform_admin` | Administra schemas/roles; no se usa desde APIs. |

El runtime configura `search_path=<schema>,public`. Las migraciones califican explícitamente objetos cross-schema de `platform_meta`; los servicios no obtienen privilegios sobre datos de negocio de otros schemas.

## Entornos y reset

- Desarrollo: una instancia local compartida, con schemas aislados.
- CI: una instancia efímera por job. Se crea, migra, siembra y destruye al finalizar.
- Nunca ejecutar `down -v`, `db-reset` o `DROP SCHEMA` contra el Postgres compartido de desarrollo.
- Todo seed es idempotente y declara en su README los IDs estables que exporta para E2E.

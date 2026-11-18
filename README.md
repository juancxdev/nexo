# Nexo

Nexo es la base de una plataforma web multi-tenant para organizaciones que necesitan publicar, administrar y operar módulos digitales independientes: eventos, convocatorias, participación, contenido, operaciones y futuras integraciones como HukID.

El proyecto nace a partir del aprendizaje del POS Restaurante, pero no queda limitado a restaurantes ni a un único tipo de operación.

## Estructura inicial

```text
nexo/
├── frontend/   # shell web + microfrontends React
├── backend/    # Core API y módulos de identidad/multitenancy
├── _bmad/      # proceso de construcción y documentación BMAD
└── _bmad-output/
```

## Dirección técnica inicial

- `frontend`: shell React responsable de sesión, tenant activo, navegación, layout, design tokens y carga de microfrontends.
- Microfrontends: módulos aislados por capacidad de negocio; cada uno posee rutas, pantallas, estado y pruebas propias, pero consume contratos compartidos del shell.
- `backend`: Core API Go/Fiber que resuelve registro, login, refresh, logout, perfil, creación/listado de tenants y navegación del sidebar. PostgreSQL es la persistencia oficial; el adaptador en memoria se conserva sólo para unit tests.
- Persistencia compartida: todos los microservicios de Nexo deben utilizar una única base PostgreSQL lógica (`nexo`) y separar su propiedad mediante schemas (`identity`, `tenants`, `onsite`, etc.). No se deben crear bases independientes por microservicio salvo una decisión arquitectónica explícita.
- Multitenancy: el tenant se deriva del contexto autenticado y de la membresía; nunca se confía en un `tenant_id` enviado libremente desde el navegador.
- Integraciones externas: se modelan detrás de puertos/adapters; HukID será un sistema separado y no compartirá base de datos ni bus interno.

## Estado

La base frontend y el Core API inicial ya están implementados. El frontend usa sesión mock temporalmente; el adaptador de sesión real contra el Core API queda como siguiente integración. Los módulos de eventos, CMS, check-in, reportes y HukID permanecen fuera del Core API.

## Integración de equipos

Antes de crear o modificar un módulo, leer [CONTRIBUTING.md](CONTRIBUTING.md) y la [guía de integración](docs/engineering/MODULE-INTEGRATION.md). El skill reutilizable para agentes está en [skills/nexo-module-integration/SKILL.md](skills/nexo-module-integration/SKILL.md).

## PostgreSQL y operación de servicios

El entorno local usa una única base lógica `nexo`. Core, Onsite, Event Operations y Event Configuration poseen schemas y roles propios, sin compartir tablas. El compose raíz es un orquestador de contenedores independientes: un cambio de un servicio se despliega de forma selectiva y no exige reconstruir los demás.

La base compartida no significa compartir tablas: cada servicio conserva la propiedad de su schema, migraciones y repositorios. La comunicación de negocio debe respetar contratos HTTP/eventos y no depender de joins contra tablas privadas de otro servicio.

Para levantar el stack compartido desde cero o después de limpiar Docker:

```bash
./scripts/dev-up.sh
```

`docker-compose.yml` contiene únicamente contenedores permanentes: una instancia PostgreSQL compartida, Core, cada microservicio, workers e infraestructura local. Las migraciones, el bootstrap y los seeds viven en `docker-compose.init.yml` y se ejecutan como jobs temporales con `docker compose run --rm`; no forman parte del runtime ni quedan como contenedores detenidos.

Para levantar sólo la infraestructura permanente, sin inicializar una base nueva:

```bash
docker compose up -d --build
```

Para reconstruir un único servicio sin tocar los demás:

```bash
docker compose up -d --build event-configuration-api
```

Para operar sólo el módulo de Eventos:

```bash
make events-up
make events-rebuild
make events-logs
```

Consulta [`backend/README.md`](backend/README.md) y [`backend/docs/openapi.yaml`](backend/docs/openapi.yaml) para los contratos disponibles.

## Contexto reutilizado desde POS

La evaluación técnica de POS está documentada en [`docs/POS-BASELINE.md`](docs/POS-BASELINE.md). CodeGraph fue inicializado en POS para explorar el shell, la sesión, el bootstrap de tenant/local/administrador, la navegación por roles y los componentes compartidos.

# Auditoría de integración — Grupo 3 Participation

Repositorio revisado: `Frknk/event-platform-architecture`, rama `main`, commit revisado `6b35455`.

## Resultado ejecutivo

El Grupo 3 entrega una base funcional y valiosa para Participation, pero no debe copiarse ni fusionarse directamente dentro de Nexo. El repositorio está construido como un paquete independiente de demostración: expone una API FastAPI/Python, mantiene un `participation-db` propio, arranca en el puerto `8000` y trae una aplicación Vite separada.

La integración correcta será una adaptación de bounded context dentro de Nexo:

```text
Nexo Core ── JWT/RBAC ── Participation API
                              │
                              ├── schema participation en PostgreSQL nexo
                              ├── role participation_app
                              ├── migrations + seed del compose raíz
                              ├── outbox/inbox de Participation
                              └── contrato consumido por Onsite y Event Configuration
```

No se integró código ejecutable automáticamente en esta revisión porque hacerlo sin resolver los contratos produciría dos bases, dos autoridades de identidad y tickets incompatibles. Esta auditoría deja la integración preparada y define exactamente qué se debe adaptar en la siguiente fase.

## Qué aporta el Grupo 3

- API de registro público con `Idempotency-Key`, control atómico de cupos y waitlist.
- Self-service de registros y cancelación.
- Postulaciones de emprendimientos/equipos, invitaciones, revisiones y adjuntos.
- PII cifrada con AES-GCM y búsqueda mediante HMAC blind index.
- Auditoría, retención, outbox/inbox y reconciliación de releases.
- Tickets QR JWS sin PII y endpoint JWKS para validación de Onsite.
- Migraciones PostgreSQL con proyecciones locales, capacidad, registros, consents, tickets y aplicaciones.
- Tests unitarios, integración PostgreSQL, E2E y prueba de concurrencia.

Estas capacidades están alineadas con el alcance funcional del producto y complementan los módulos ya presentes en Nexo.

## Conflictos que deben resolverse antes de integrar

### 1. Base de datos y despliegue

El compose del Grupo 3 levanta `participation-db`, base `participation_db`, usuario `participation_app` y puerto `5434`. Nexo ya tiene una única base lógica `nexo`, roles separados y jobs de migración en `docker-compose.init.yml`.

Adaptación obligatoria:

- eliminar el compose de PostgreSQL propio del runtime;
- crear el schema propietario `participation` dentro de `nexo`;
- registrar una tabla de migraciones exclusiva, por ejemplo `platform_meta.participation_schema_migrations`;
- crear el rol `participation_app` con `USAGE` y privilegios únicamente sobre `participation`;
- ejecutar migraciones y seeds como jobs temporales del compose raíz;
- conservar UUID opacos para referencias a `events`, `identity`, `onsite` y `operations`, sin foreign keys cross-service.

### 2. Autoridad de autenticación

El contrato del Grupo 3 habla de tokens con `organization_id`, mientras Nexo Core emite `tenant_id`, roles, scopes, `iss`, `aud`, `sid`, `jti` y `kid`. Participation no debe firmar ni aceptar una sesión administrativa paralela.

Adaptación obligatoria:

- usar el JWT emitido por Core para backoffice;
- mapear `tenant_id` de Nexo a `organization_id` únicamente dentro del adaptador de entrada, sin cambiar el ownership interno del dominio;
- validar firma mediante el JWKS de Core, issuer, audience, expiración, `nbf`, tenant y scopes;
- mantener tokens de acceso del participante separados del JWT administrativo;
- eliminar `X-Mock-*` del flujo productivo y limitarlo a `development/test`.

### 3. QR y Onsite

El Grupo 3 propone un JWS `ES256` firmado por Participation con claims `jti`, `iss`, `aud=onsite`, `org_id`, `event_id`, `reg_id`, `scope`, `ver`, `iat` y `exp`. El Onsite actual todavía busca tickets por identificador y proyección local; no consume ese JWS como contrato principal.

Adaptación obligatoria:

- conservar el JWS del Grupo 3 como contrato canónico de ticket;
- publicar JWKS propio de Participation para validación criptográfica de Onsite;
- añadir al Onsite un adapter ES256/JWKS con cache y rotación de claves;
- validar `aud`, organización/tenant, evento, scope, expiración, versión, JTI y revocación;
- mantener la proyección local `onsite.ticket_projections` como lectura operacional, alimentada por eventos de Participation;
- conservar búsqueda manual como fallback operativo, nunca como sustituto de la validación criptográfica del QR.

### 4. Event Configuration

El Grupo 3 consume `configuration.event.release_published.v1` y `configuration.event.cancelled.v1`. Event Configuration ya documenta esos límites, pero el adaptador real de Participation todavía no está presente en Nexo.

Adaptación obligatoria:

- materializar releases publicados en `participation.release_projections`;
- consumir forms, bindings, consentimientos y policy de capacidad desde la release publicada;
- usar `inbox` con deduplicación por `domain_event_id`;
- rechazar releases incompletas o desactualizadas;
- cortar nuevas inscripciones al recibir cierre/cancelación;
- emitir eventos de registro, waitlist, ticket y postulación siguiendo el envelope canónico.

### 5. Frontend

`apps/participation-web` es una aplicación Vite independiente, sin integración con el shell ni el design system de Nexo. Nexo exige microfrontends bajo `frontend/src/microfrontends/<module-id>/`, registro en el manifest, `ModuleContext`, i18n ES/EN y componentes del sistema de diseño.

Adaptación obligatoria:

- convertir la aplicación en uno o más microfrontends dentro del shell Nexo;
- separar registro público de backoffice/self-service si los límites de sesión lo requieren;
- reemplazar estilos locales por exports del design system;
- registrar rutas y capacidades sin tocar rutas globales del shell;
- consumir `context.apiBaseUrl` y el cliente HTTP compartido;
- añadir estados loading, empty, error, forbidden y API no disponible;
- conservar las pruebas E2E del Grupo 3, adaptando fixtures y URLs al stack raíz.

## Mapa de integración con los módulos actuales

| Área del Grupo 3 | Destino en Nexo | Decisión |
|---|---|---|
| Participation API | `microservices/participation` | Adaptar runtime y estructura; no copiar compose de DB |
| Participation schema | PostgreSQL `nexo`, schema `participation` | Integrar con migrador raíz y rol propio |
| Forms/releases | Event Configuration | Consumir por contrato/eventos; no leer `events.*` |
| Tickets/JWS | Participation → Onsite | Participation firma; Onsite valida y proyecta |
| Registro/postulación | Participation | Ownership exclusivo del Grupo 3 |
| Certificado subject | Event Operations | Emitir evento sin PII; HukID recibe claims con consentimiento |
| Participation web | `frontend/src/microfrontends/participation` | Reutilizar shell, tokens e i18n |
| Outbox/inbox | Plataforma compartida por contrato | Mantener ownership local y replay seguro |

## Riesgos detectados en el repositorio externo

- CORS está configurado como `*` con credenciales; debe restringirse en Nexo.
- Hay fallback automático a SQLite si PostgreSQL no está disponible; en el runtime Nexo no debe ocultarse una caída de PostgreSQL con otra persistencia.
- Las claves criptográficas locales tienen defaults deterministas para demo; deben ser obligatorias en producción y gestionadas como secretos.
- El compose externo no es el compose raíz y no incluye Core, Event Configuration, Onsite, Operations ni HukID.
- La aplicación web externa no usa el contrato `ModuleContext` ni el design system del shell Nexo.
- El README externo todavía describe certificado PDF como parte del MVP; Nexo ya decidió que la entrega verificable se delega a HukID.

## Criterio de integración

La implementación futura debe hacerse en este orden:

1. Congelar y versionar los contratos Participation/OpenAPI/eventos.
2. Adaptar persistencia al schema único `participation`.
3. Integrar autenticación Core y autorización por tenant/scope.
4. Integrar consumer de releases de Event Configuration.
5. Integrar emisión JWKS/JWS y adapter de Onsite.
6. Integrar el microfrontend con el shell y design system.
7. Añadir seeds al entorno raíz y E2E punta a punta.
8. Activar el check `microservices-quality` y los contract tests de Participation.

La siguiente implementación debe partir de este mapeo, no del `docker-compose.yml` ni de la base `participation_db` del repositorio externo.

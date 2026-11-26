# Participation API

Servicio público de formularios, registros y tickets de participación. Usa el esquema `participation` dentro de la base PostgreSQL central `nexo`; no levanta una base propia.

## Desarrollo

```bash
docker compose -f docker-compose.yml up -d postgres
docker compose -f docker-compose.init.yml run --rm migrate-participation
docker compose -f docker-compose.init.yml run --rm seed-participation
docker compose up -d participation-api
```

El ticket de desarrollo es un JWS de fixture. La validación criptográfica definitiva se conectará al emisor de Participación/Onsite cuando publique su `JWKS`.

## Protección PA-040: v1 y v2

Las rutas públicas v1 se mantienen sin cambios de contrato y **no exigen challenge ni aplican los límites PA-040**. La protección completa de registro está en las rutas públicas v2, que usa el microfrontend Participation:

- `GET /participation/public/v2/sites/{site}/events/{event}/form`
- `GET /participation/public/v2/sites/{site}/events/{event}/challenge`
- `POST /participation/public/v2/sites/{site}/events/{event}/registrations`

v2 limita las solicitudes por IP, evento y dispositivo. El dispositivo se identifica con una cookie aleatoria firmada por el servidor (`HttpOnly`, `SameSite=Lax`); no se aceptan IDs de dispositivo proporcionados libremente por el cliente. La IP proviene de la conexión directa, salvo que el peer coincida con una red configurada como proxy confiable. Sólo en ese caso se interpreta `X-Forwarded-For`, recorriendo la cadena de proxies confiables desde el peer hacia el cliente. Respuestas limitadas incluyen `429` y `Retry-After`.

El challenge es un proof-of-work SHA-256: el servidor firma un token ligado al sitio, evento y cookie del dispositivo; el cliente busca una prueba que cumpla la dificultad indicada. El token expira y sólo puede consumirse una vez por instancia. El frontend resuelve el challenge antes de enviar el registro y conserva el payload y la respuesta pública de registro.

Configuración por variables de entorno (valores por defecto entre paréntesis):

| Variable | Uso |
| --- | --- |
| `PARTICIPATION_RATE_LIMIT_WINDOW` (`1m`) | Ventana fija de los contadores; admite duraciones Go. |
| `PARTICIPATION_RATE_LIMIT_IP` (`120`) | Solicitudes por IP y ventana. |
| `PARTICIPATION_RATE_LIMIT_EVENT` (`1200`) | Solicitudes por sitio/evento y ventana. |
| `PARTICIPATION_RATE_LIMIT_DEVICE` (`20`) | Solicitudes por dispositivo firmado y ventana. |
| `PARTICIPATION_SECURITY_ALLOWLIST_CIDRS` | CIDRs o IPs separados por comas que omiten rate limit y challenge de v2. Vacío por defecto. |
| `PARTICIPATION_TRUSTED_PROXY_CIDRS` | CIDRs/IPs de proxies cuya conexión permite interpretar `X-Forwarded-For`. Vacío por defecto. |
| `PARTICIPATION_CHALLENGE_DIFFICULTY` (`14`) | Bits cero iniciales requeridos en SHA-256; rango permitido 8–16 para mantener resoluble el flujo web. |
| `PARTICIPATION_CHALLENGE_TTL` (`5m`) | Vigencia del challenge; rango permitido 1s–15m. |
| `PARTICIPATION_CHALLENGE_SECRET` | Base64 de al menos 32 bytes aleatorios. Si falta, se genera una clave efímera al iniciar. |
| `PARTICIPATION_DEVICE_COOKIE_SECURE` | Fuerza el atributo `Secure`; por defecto se activa salvo en `APP_ENV=development` o `test`. |
| `PARTICIPATION_DEVICE_COOKIE_SAMESITE` (`lax`) | `lax`, `strict` o `none`; `none` requiere `PARTICIPATION_DEVICE_COOKIE_SECURE=true` para frontends realmente cross-site. |
| `PARTICIPATION_SECURITY_METRICS_TOKEN` | Bearer token de al menos 32 caracteres para consultar los contadores agregados. Sin configurar, el endpoint devuelve 503. |

`GET /participation/internal/v1/security/metrics` requiere ese token y sólo devuelve contadores agregados; no publica IPs, cookies, IDs de evento, pruebas ni datos de participantes. Las claves de bucket son HMAC no reversibles y viven sólo en memoria.
Los cambios de allowlist, redes confiables y límites se aplican al reiniciar el servicio.

### Limitaciones de despliegue

Los contadores de rate limit y los IDs de challenges consumidos son locales al proceso. Para la protección PA-040 completa, ejecutar una sola réplica. Con varias réplicas, fijar afinidad de cliente y configurar el mismo `PARTICIPATION_CHALLENGE_SECRET` no resuelve el replay entre instancias ni comparte los límites: se requiere un almacén compartido con consumo atómico y rate limiting distribuido. Esta implementación no modifica ni utiliza la base compartida para ese estado. Si `X-Forwarded-For` llega desde un proxy no incluido en `PARTICIPATION_TRUSTED_PROXY_CIDRS`, se ignora y se limita por la IP del proxy.

## Cancelación, waitlist y rendimiento PA-041

### Cancelación self-service y promoción de lista de espera

- `POST /participation/self/v1/registrations/{id}/cancel` exige `Authorization: Bearer <self_access_token>` asociado al registro (responde `401` sin token y `404` si el token no corresponde al registro).
- Es idempotente: cancelar un registro ya `cancelled` devuelve `200` sin alterar cupos ni lista de espera.
- En una única transacción PostgreSQL con bloqueo `FOR UPDATE` sobre `participation.forms` seguido de `participation.registrations`:
  - Marca el registro como `cancelled` y revoca su ticket (`ticket_state = 'revoked'`).
  - Si estaba `confirmed` y existe un participante en `waitlisted`, promueve al de menor `waitlist_position` a `confirmed` con un nuevo `ticket_id` y JWS Ed25519 activo.
  - Si no hay lista de espera, libera el cupo (`remaining + 1`, `confirmed_count - 1`).

### Configuración del pool de PostgreSQL (`pgxpool`)

Variables opcionales en `cmd/api/main.go` (si no se definen, aplican los defaults de `pgxpool`):

| Variable | Uso |
| --- | --- |
| `PARTICIPATION_DB_MAX_CONNS` | Máximo de conexiones abiertas en el pool. |
| `PARTICIPATION_DB_MIN_CONNS` | Mínimo de conexiones inactivas mantenidas. |
| `PARTICIPATION_DB_MAX_CONN_LIFETIME` | Tiempo máximo de vida de una conexión (duración Go). |
| `PARTICIPATION_DB_MAX_CONN_IDLE_TIME` | Tiempo máximo de inactividad antes de cerrar la conexión. |

### Pruebas de concurrencia y perfil de carga 3×

Las pruebas de integración y carga viven en `internal/httpapi/load_integration_test.go` y se activan sólo cuando `PARTICIPATION_TEST_DATABASE_URL` apunta a un PostgreSQL aislado (no afectan `go test ./...` en CI sin base de datos).

- **Supuesto de pico base**: al no existir un valor histórico de tráfico productivo en el repositorio, se define como pico base de ensayo **100 operaciones**; el perfil **3×** ejecuta **300 operaciones mixtas** (registro, waitlist y cancelación) con **60 goroutines concurrentes** sobre un evento de capacidad `60` y `30` registros pre-sembrados.
- **Resultados medidos sobre `postgres:16-alpine` aislado** (`0` fallos, `0` sobrecupos, `60` tickets activos únicos):

| Pool (`MaxConns`) | Throughput | `p50` | `p95` | `p99` | `acquire_duration` acumulado |
| --- | --- | --- | --- | --- | --- |
| Default (`12`) | `221.36 req/s` | `246.444ms` | `346.178ms` | `383.152ms` | `59.013s` (`empty_acquire=599`) |
| Ajustado (`20`) | `231.36 req/s` | `241.837ms` | `304.034ms` | `345.891ms` | `45.775s` (`empty_acquire=600`) |

Para reproducir en un contenedor aislado sin tocar la base compartida, iniciar `postgres:16-alpine`, aplicar `migrations/000001_participation.up.sql` a `000003_hukid_credentials.up.sql` y ejecutar `go test -v ./...` con `PARTICIPATION_TEST_DATABASE_URL`.

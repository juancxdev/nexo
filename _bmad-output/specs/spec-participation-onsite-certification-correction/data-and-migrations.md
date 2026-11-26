# Modelo de datos y migraciones

## 1. Principios

- Base única `nexo` por ambiente; `nexo_test` en CI.
- Esquemas propietarios: `identity`, `event_configuration`, `participation`, `onsite`, `operations`, `certificates`, `integration`.
- Cada API usa un rol restringido a su esquema y a contratos explícitos.
- El rol migrador es distinto al rol de runtime.
- Sin foreign keys entre bounded contexts.
- Migraciones sin seeds; los seeds se ejecutan explícitamente.

## 2. Participation

### Cambios requeridos

Agregar a registros:

```sql
email_normalized text NOT NULL;
participant_fingerprint text NULL;
request_hash text NOT NULL;
supersedes_registration_id uuid NULL;
```

Índices:

```sql
CREATE UNIQUE INDEX registrations_unique_email_per_form
ON participation.registrations (tenant_id, form_id, email_normalized)
WHERE status IN ('pending', 'confirmed', 'waitlisted');

CREATE UNIQUE INDEX registrations_unique_idempotency
ON participation.registrations (tenant_id, form_id, idempotency_key);
```

La restricción antigua basada en `email` sin normalizar se reemplaza después del backfill y validación.

### Migración segura

1. Añadir columnas nullable.
2. Calcular `email_normalized` para históricos.
3. Detectar grupos duplicados y generar reporte de conciliación; no eliminarlos automáticamente.
4. Marcar un registro canónico y bloquear nuevas emisiones para duplicados pendientes de revisión.
5. Establecer columnas requeridas para nuevos registros.
6. Crear índice único parcial de forma concurrente cuando el motor/runner lo permita.
7. Retirar la restricción anterior.

## 3. Proyección de eventos en Onsite

La tabla de eventos operacionales debe contener:

```text
id interno
tenant_id
source_event_id canónico
name
status
starts_at / ends_at / timezone
published_revision
registration_form_id
synchronized_at
```

Restricción única: `(tenant_id, source_event_id)`. Un evento con revisión menor no sobrescribe una revisión mayor.

Eliminar del frontend y seeds cualquier decisión basada en slugs `demo`, `fiis`, `event-demo` o `event-fiis`. Los slugs solo pueden ser atributos de búsqueda, nunca identidad relacional.

## 4. Padrón de Onsite

Crear `onsite.participant_roster` como proyección propiedad de Onsite:

```text
id
tenant_id
event_id interno
source_event_id
registration_id
ticket_id
registration_code
display_name
registration_status
effective_attendance_state
checked_in_at
checked_out_at
last_attendance_fact_id
source_revision
synchronized_at
created_at / updated_at
```

Restricciones:

- único `(tenant_id, source_event_id, registration_id)`;
- único ticket activo por evento cuando corresponda;
- toda consulta incluye `tenant_id`;
- el estado se actualiza en la misma transacción que inserta un attendance fact;
- la proyección es reconstruible desde eventos/inbox y facts locales.

El nombre se almacena por necesidad operativa. Correo, documento y respuestas no se copian por defecto.

## 5. Asistencia

`onsite.attendance_facts` permanece append-only y añade o garantiza:

```text
tenant_id
event_id
registration_id
ticket_id
fact_type: check_in | check_out | void
compensates_fact_id
device_id
shift_id
scope_id
occurred_at
recorded_at
actor_id
reason_code
idempotency_key
request_hash
```

Índices y locking deben permitir serializar la mutación por `(tenant_id,event_id,registration_id,scope_id)`. La implementación puede usar advisory lock transaccional o una fila de estado bloqueada con `SELECT ... FOR UPDATE`; el criterio es que dos solicitudes concurrentes dejen una sola presencia efectiva.

## 6. Reglas y evidencia de certificado

### `certificates.eligibility_rules`

Campos mínimos:

```text
id, tenant_id, event_id, version, status
require_event_finished boolean
minimum_checkins integer
minimum_sessions_attended integer nullable
minimum_attendance_ratio numeric nullable
required_session_ids jsonb
require_checkout boolean
issuance_mode manual | automatic
published_at, created_by, created_at
```

Una versión `published` es inmutable. Único `(tenant_id,event_id,version)`.

### `certificates.eligibility_snapshots`

```text
id, tenant_id, event_id, registration_id
rule_id, rule_version
eligible, blocked_reasons jsonb
attendance_watermark
evidence jsonb cifrado o minimizado
evidence_hash
evaluated_at
```

### `certificates.certificates`

Debe evitar dos certificados activos para `(tenant_id,event_id,registration_id,rule_version,credential_type)`. Revocación y reemplazo mantienen historial.

## 7. Entregas HukID

`integration.hukid_deliveries` guarda antes de la llamada:

- certificate_id;
- issuer y definición versionada;
- claims cifrados AES-256-GCM;
- versión de clave y nonce;
- canonical request hash;
- claims hash;
- consentimiento;
- idempotency key;
- source reference;
- estado, intentos y siguiente intento;
- HukID request/credential ID;
- oferta cifrada y expiración.

Nunca se persiste una segunda variante del request para una misma idempotency key.

## 8. Seed determinista

Actualizar `demo-day-full` para crear:

- tenant IncubaUNAS;
- evento publicado en curso y evento finalizado;
- formulario publicado para cada evento;
- participante confirmado sin check-in;
- participante presente;
- participante con checkout;
- participante cancelado;
- participante no-show en evento finalizado;
- intento de duplicado por mayúsculas/espacios;
- tickets válido, revocado, expirado y de otro evento;
- regla de certificado publicada;
- participante elegible y bloqueado por tiempo;
- certificado y delivery HukID en varios estados.

El reset de seed solo existe en development/test. Cada E2E recibe datos reproducibles y no hereda un `already_present` de una ejecución anterior.

## 9. Despliegue y rollback

- Las migraciones son compatibles hacia atrás durante al menos una versión de API.
- Primero se despliegan columnas/proyecciones, después productores y consumidores, y al final se activan restricciones.
- Un feature flag controla el nuevo roster y la nueva elegibilidad durante la transición.
- El rollback desactiva consumo/UI nueva, pero no elimina hechos ni columnas con datos.
- La conciliación histórica debe producir métricas de registros duplicados, eventos no mapeados y asistencias huérfanas antes de activar la ruta nueva.


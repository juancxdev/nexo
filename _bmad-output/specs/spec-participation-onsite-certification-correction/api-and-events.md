# Contratos API y eventos

Todos los endpoints autentican JWT de Nexo Core, infieren `tenant_id` del contexto firmado y rechazan un tenant recibido libremente en el body. Los errores usan RFC 9457.

## 1. Participation

### `POST /participation/v1/public/forms/{public_slug}/submissions`

Headers:

```http
Idempotency-Key: <uuid-or-ulid>
Content-Type: application/json
```

Comportamiento adicional:

- normaliza correo antes de cualquier consulta;
- calcula hash canónico del request;
- reserva unicidad en transacción;
- emite un único ticket/credencial de entrada;
- publica el registro confirmado mediante outbox.

Respuestas relevantes:

- `201`: nuevo registro.
- `200`: replay idempotente de una creación anterior.
- `409 already_registered`: mismo formulario y correo normalizado con key distinta.
- `409 idempotency_key_reused`: misma key con payload distinto.
- `422 validation_failed`: campos inválidos.

Ejemplo de conflicto seguro:

```json
{
  "type": "https://errors.nexo.local/already-registered",
  "title": "already_registered",
  "status": 409,
  "detail": "Ya existe una inscripción para este evento con el correo indicado.",
  "code": "already_registered",
  "recovery_available": true,
  "trace_id": "01..."
}
```

## 2. Catálogo operacional de Onsite

### `GET /onsite/v1/events`

Scope: `onsite:read`.

Query opcional: `status=upcoming,active,finished`, `cursor`, `limit`.

```json
{
  "items": [
    {
      "id": "uuid-interno-onsite",
      "source_event_id": "uuid-canonico",
      "name": "Congreso de Innovación y Demo Day 2026",
      "status": "active",
      "starts_at": "2026-11-24T14:00:00Z",
      "ends_at": "2026-11-24T22:00:00Z",
      "timezone": "America/Lima",
      "published_revision": 4,
      "roster_count": 128,
      "present_count": 31
    }
  ],
  "next_cursor": null
}
```

No se devolverán eventos de otro tenant ni borradores no operables.

## 3. Padrón y asistencia

### `GET /onsite/v1/events/{event_id}/roster`

Scope: `onsite:read`.

Query: `state`, `q`, `scope`, `cursor`, `limit`.

```json
{
  "items": [
    {
      "registration_id": "uuid",
      "ticket_id": "uuid",
      "display_name": "Juan Campos",
      "registration_code": "NX-REG-000123",
      "state": "confirmed",
      "checked_in_at": null,
      "checked_out_at": null,
      "certificate": {
        "eligible": false,
        "blocked_reasons": ["event_not_finished"],
        "status": "not_issued"
      }
    }
  ],
  "summary": {
    "confirmed": 97,
    "present": 31,
    "checked_out": 0,
    "no_show": 0,
    "cancelled": 0
  },
  "watermark": "01...",
  "next_cursor": null
}
```

PII adicional solo se entrega si el scope y la política del evento lo permiten.

### Escrituras existentes endurecidas

- `POST /onsite/v1/attendance/check-ins`
- `POST /onsite/v1/attendance/check-outs`
- `POST /onsite/v1/attendance/facts/{fact_id}/voids`

Todas requieren `Idempotency-Key`. El body referencia `event_id`, `ticket_token` o `ticket_id`, `device_id`, `shift_id` y `scope_id` según corresponda. El servidor resuelve tenant y valida la relación completa.

Errores mínimos: `ticket_invalid`, `ticket_expired`, `ticket_revoked`, `wrong_event`, `wrong_scope`, `device_revoked`, `already_present`, `not_present`, `idempotency_key_reused`.

## 4. Elegibilidad y certificados

### `GET /operations/v1/events/{event_id}/certificate-eligibility`

Scope: `certificates:issue` o `reports:read`.

Filtros: `registration_id`, `eligible`, `cursor`, `limit`.

Devuelve la versión de regla, evidencia resumida, elegibilidad y razones de bloqueo.

### `POST /operations/v1/certificate-jobs`

Scope: `certificates:issue`.

```json
{
  "event_id": "uuid",
  "registration_ids": ["uuid"],
  "rule_version": 1,
  "delivery": "hukid"
}
```

Reglas:

- reevalúa elegibilidad dentro de la transacción;
- congela evidencia;
- crea una sola emisión activa por registro, evento y versión de regla;
- devuelve `422 certificate_not_eligible` con razones por participante;
- `Idempotency-Key` obligatorio.

### `POST /operations/v1/certificates/{certificate_id}/hukid-deliveries`

El body solo contiene consentimiento o su referencia:

```json
{
  "consent_id": "uuid"
}
```

El servidor determina issuer, definición, versión y claims. Si la oferta ya existe, reproduce el delivery. Si expiró, se crea un reintento controlado sin duplicar el certificado.

## 5. Eventos de integración

Todo evento incluye `event_id`, `event_type`, `occurred_at`, `producer`, `schema_version`, `tenant_id`, `correlation_id`, `causation_id` y payload. Los consumidores mantienen inbox idempotente.

### `events.event.published.v1`

```json
{
  "event_id": "uuid",
  "name": "Congreso de Innovación y Demo Day 2026",
  "status": "published",
  "starts_at": "2026-11-24T14:00:00Z",
  "ends_at": "2026-11-24T22:00:00Z",
  "timezone": "America/Lima",
  "revision": 4,
  "registration_form_id": "uuid"
}
```

### `participation.registration.confirmed.v1`

```json
{
  "registration_id": "uuid",
  "event_id": "uuid",
  "ticket_id": "uuid",
  "registration_code": "NX-REG-000123",
  "display_name": "Juan Campos",
  "status": "confirmed",
  "occurred_at": "2026-11-24T15:00:00Z"
}
```

No incluye respuestas completas del formulario ni correo salvo que exista un contrato explícito y cifrado para un caso autorizado.

### `participation.registration.cancelled.v1`

Incluye `registration_id`, `event_id`, `ticket_id`, `reason_code` y timestamp; Onsite deriva `cancelled` y bloquea check-in.

### `onsite.attendance.changed.v1`

Incluye `registration_id`, `event_id`, `effective_state`, `fact_id`, `fact_type`, `occurred_at` y `watermark`. Event Operations lo usa para recalcular elegibilidad.

### `operations.certificate.eligibility.changed.v1`

Incluye `registration_id`, `event_id`, `rule_version`, `eligible`, `blocked_reasons` y `evidence_hash`.

### `operations.certificate.issued.v1`

Incluye identificadores internos, `verification_code`, `evidence_hash` y estado; nunca claims completos ni oferta HukID.

## 6. Reconciliación

Hasta disponer de broker, los eventos se publican mediante outbox PostgreSQL y polling con `FOR UPDATE SKIP LOCKED`. Cada proyección debe ofrecer un comando interno de reconciliación/backfill, protegido y auditable, para reconstruir eventos o registros perdidos sin insertar duplicados.


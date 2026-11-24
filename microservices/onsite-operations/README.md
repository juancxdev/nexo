# Onsite Operations

Servicio independiente de operación presencial para Nexo. Es dueño de sus proyecciones, hechos append-only, idempotencia y auditoría; no consulta tablas privadas del Core API.

## Regla de persistencia y pendiente

Este corte vertical usa temporalmente la base lógica `nexo_onsite` para facilitar el desarrollo aislado. Antes de pasar a un entorno integrado debe consolidarse en la base PostgreSQL única `nexo`, dentro del schema `onsite`, compartida por los microservicios de Nexo.

La consolidación no implica compartir tablas entre servicios: cada servicio conserva la propiedad de sus schemas, migraciones y repositorios. La comunicación entre bounded contexts seguirá realizándose mediante contratos HTTP/eventos.

## Demo local

Requiere Docker, `migrate`, `psql` y Go 1.27:

```bash
make db-up
make db-migrate db-seed
make run
```

La API escucha en `http://localhost:8082`. El seed es repetible y contiene Demo Day IncubaUNAS. En desarrollo/test se aceptan headers mock (`X-Mock-Actor`, `X-Mock-Role`, `X-Mock-Tenant`, `X-Mock-Device`); fuera de esos perfiles se rechazan.

Rutas principales: `/health/live`, `/health/ready`, `/onsite/v1/readiness`, `/onsite/v1/tickets/search`, `/onsite/v1/attendance/check-ins`, `/onsite/v1/attendance`, `/onsite/v1/dashboard` y `/onsite/v1/verification-sessions`.

La verificación HukID usa el contrato OID4VP del nodo Grupo 5. Onsite persiste el
contexto tenant/evento/scope/ticket de la sesión y solo registra asistencia al
observar un resultado `completed/pass`. En producción se exige presentar el
`verification_session_id` al endpoint de check-in.

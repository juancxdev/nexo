# Rotación controlada de claves JWT

Nexo firma tokens con RSA (`RS256`) y publica las claves públicas en `GET /.well-known/jwks.json`. El Core mantiene un anillo de claves persistente: una clave `active` firma tokens nuevos y las claves `retiring` sólo verifican tokens emitidos antes de la rotación.

## Secuencia operativa

1. Generar una clave nueva, con un `kid` único.
2. Marcar la clave actual como `retiring` y registrar su `retire_after`.
3. Publicar ambas claves en JWKS.
4. Firmar tokens nuevos con el nuevo `kid`.
5. Los microservicios aceptan la anterior hasta `retire_after`.
6. Tras la ventana de transición, la clave anterior deja de publicarse y de verificar tokens.

Esto evita invalidar sesiones activas durante un despliegue. La ventana debe cubrir como mínimo el tiempo de vida máximo de los access tokens; en producción se recomienda una ventana mayor que `JWT_TTL + margen de propagación JWKS`.

## Operación local

El endpoint es exclusivamente interno y exige un secreto independiente del JWT:

```bash
curl -X POST http://localhost:8081/internal/v1/security/signing-keys/rotations \
  -H 'X-Nexo-Rotation-Token: nexo-local-rotate-only' \
  -H 'Content-Type: application/json' \
  -d '{"transition_window_seconds": 32400}'
```

La respuesta incluye el nuevo `kid`. Luego se valida que JWKS exponga exactamente la clave activa y la clave en transición.

## Producción

- `NEXO_JWT_KEY_RING_DIR` debe ser una proyección con permisos `0600` de un secret manager o KMS; no se persiste en la imagen ni en PostgreSQL.
- `NEXO_INTERNAL_ROTATION_TOKEN` y `HUKID_CLAIMS_ENCRYPTION_KEY` deben inyectarse mediante el gestor de secretos del entorno y rotarse fuera de Git.
- El endpoint de rotación debe estar aislado de Internet (mTLS/VPN/red administrativa) además del token de operación.
- Antes de retirar una clave, confirmar que no hay access tokens válidos firmados con ese `kid` y que los caches JWKS de los consumidores expiran dentro de la ventana.
- Para una rotación de emergencia, revocar el `kid` comprometido y revocar las sesiones afectadas: retirar una clave antes de su ventana invalida deliberadamente esos tokens.

## HukID y certificados

Nexo no genera PDF ni credenciales verificables. Crea un registro de evidencia y, tras consentimiento explícito, congela el payload canónico de integración, lo cifra con AES-256-GCM y lo encola para HukID. El worker reutiliza exactamente el mismo payload e `Idempotency-Key` ante reintentos. Sólo HukID emite la credencial y gestiona OID4VCI.

# Contratos consumidos de HukID

Nexo consume HukID como un sistema externo. Estos contratos se versionan en Nexo
para que una actualización del nodo no cambie silenciosamente el adaptador.

## Emisión

- OAuth2 Client Credentials: `POST /oauth2/token`.
- Solicitud: `POST /v1/issuance-requests` con `Idempotency-Key`.
- Consulta: `GET /v1/issuance-requests/{id}`.
- Cancelación: `POST /v1/issuance-requests/{id}/cancellations`.
- Scopes: `issuance:create issuance:read`.
- Delivery soportado por HukID Grupo 5: `pre-authorized_code` sin `txCodeRequired`.

## Verificación OID4VP

El nodo Grupo 5 expone la superficie administrativa de verificación:

- `POST /v1/operator/tenants/{tenantId}/verification-sessions`.
- `GET /v1/operator/tenants/{tenantId}/verification-sessions/{sessionId}`.

La creación devuelve únicamente `sessionId`, `launchUri` y `expiresAt`. La wallet
escanea `launchUri`; el nodo nunca devuelve a Nexo el JWS, el `vp_token`, el nonce
ni el estado interno OID4VP. Nexo solo consume `status`, `result`, `codes` y el
digest del reporte.

## Límites de integración

Nexo no comparte la base de datos de HukID, no implementa OID4VCI/OID4VP y no
guarda credenciales completas. Los identificadores de issuer y credential
definition se obtienen de configuración del tenant, nunca del navegador.

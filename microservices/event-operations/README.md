# Event Operations

API y worker del Grupo 4 para reportes, certificados y entregas HukID. Usa el
schema `operations`, `certificates` e `integration` dentro de la única base
PostgreSQL `nexo`. El worker no genera credenciales verificables: llama a la
Integration API de HukID, guarda el identificador externo y reconcilia por
polling.

En desarrollo se ejecuta desde el compose raíz en `:8083`. La documentación
HTTP está en `docs/openapi.yaml`.

El servicio maneja dos tipos de entrega HukID: `participation_ticket` para el
boleto del evento y `attendance_certificate` para el certificado. El payload se
congela y cifra antes del worker; ambos usan `pre-authorized_code` sin `tx_code`.
El nodo real se configura por `integration.hukid_configs`; el simulador solo se
usa en desarrollo/test.

## HukID real o sandbox

El worker acepta overrides de entorno para conectar el sandbox o nodo real sin
guardar secretos en PostgreSQL ni en el repositorio:

- `HUKID_BASE_URL`: base de Integration API, normalmente terminada en `/v1`.
- `HUKID_OAUTH_URL`: endpoint OAuth2 de `client_credentials`.
- `HUKID_ISSUER_ID`, `HUKID_CREDENTIAL_DEFINITION` y
  `HUKID_PARTICIPATION_CREDENTIAL_DEFINITION`.
- `HUKID_CLIENT_ID` y `HUKID_CLIENT_SECRET`, enviados mediante HTTP Basic en
  el token endpoint cuando ambos están presentes.
- `HUKID_CA_CERT_FILE`, `HUKID_CLIENT_CERT_FILE` y `HUKID_CLIENT_KEY_FILE`
  para CA privada o mTLS, si el contrato de HukID lo exige.

En producción el endpoint debe ser HTTPS. El cliente no habilita
`InsecureSkipVerify`. Si el entorno no requiere mTLS, las variables de
certificado se dejan vacías. Ver `.env.hukid.example` en la raíz.

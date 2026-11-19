# Event Operations

API y worker del Grupo 4 para reportes, certificados y entregas HukID. Usa el
schema `operations`, `certificates` e `integration` dentro de la única base
PostgreSQL `nexo`. El worker no genera credenciales verificables: llama a la
Integration API de HukID, guarda el identificador externo y reconcilia por
polling.

En desarrollo se ejecuta desde el compose raíz en `:8083`. La documentación
HTTP está en `docs/openapi.yaml`.

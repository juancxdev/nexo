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

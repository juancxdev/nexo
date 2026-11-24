---
title: 'Integración de Nexo con el nodo HukID del Grupo 5'
type: 'feature'
created: '2026-11-24'
status: 'in-review'
route: 'dispatch'
review_loop_iteration: 0
baseline_commit: 'eb8e31ab9bcfb60716b05e3acd029541c31a3f81'
context:
  - '/Users/juancx/Documents/CX-Developments/SELF/nexo/docs/engineering/GROUP3-INTEGRATION-AUDIT.md'
  - '/Users/juancx/Documents/CX-Developments/SELF/nexo/_bmad-output/implementation-artifacts/spec-group4-operations-complete.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Nexo necesita emitir dos credenciales verificables —boleto de participación y certificado de asistencia— y verificar el boleto durante el acceso presencial sin incorporar criptografía, OID4VCI, OID4VP ni almacenamiento de credenciales dentro de sus propios dominios. El adaptador HukID actual solo cubre parcialmente certificados, depende del simulador y envía un payload incompatible con el nodo del Grupo 5 mediante `txCodeRequired: true`.

**Approach:** Mantener el backend Go del Grupo 5 como nodo HukID independiente y desplegable. Nexo lo consumirá mediante dos adaptadores: emisión, utilizado por Registration/Event Operations para boletos y certificados; y verificación, utilizado por Onsite Operations para sesiones OID4VP. HukID comprobará autenticidad y estado de la credencial, mientras Nexo conservará las reglas de elegibilidad, evento, scope, idempotencia y registro transaccional del check-in.

## Boundaries & Constraints

**Always:** Usar OAuth2 Client Credentials, HTTPS, OpenAPI canónico, `Idempotency-Key`, polling/reconciliación y referencias opacas. Separar `EventParticipationCredential` de `EventAttendanceCertificateCredential`. El check-in solo se registra después de una presentación válida y de validar en Nexo tenant, evento, registro, ticket, scope, vigencia operativa y ausencia de presencia efectiva. Mantener claims congelados y cifrados antes del primer intento. Desplegar el simulador únicamente en desarrollo/test y permitir configurar el nodo real mediante secretos y variables de entorno.

**Never:** Compartir base de datos entre Nexo y HukID, copiar tablas o lógica criptográfica de HukID, generar VC dentro de Nexo, guardar credenciales completas, aceptar issuer/definition/claims desde el navegador, tratar el QR de ticket JWS anterior como credencial HukID, registrar check-in solo porque la firma sea válida o habilitar endpoints `__test` en producción.

## I/O & Edge-Case Matrix

| Escenario | Entrada / estado | Resultado esperado | Manejo de error |
|---|---|---|---|
| Emisión de boleto | inscripción confirmada | oferta OID4VCI para `EventParticipationCredential` | replay con la misma clave y payload |
| Presentación en acceso | sesión OID4VP válida y consentimiento | resultado verificado; Nexo registra check-in una sola vez | rechazo por evento/scope/ticket/replay |
| Emisión de certificado | asistencia elegible | oferta OID4VCI para `EventAttendanceCertificateCredential` | 422 permanente; 429/503 reintentable |
| Credencial inválida | revocada, expirada o holder binding incorrecto | no se registra presencia | error específico y auditable sin PII |
| Concurrencia | dos verificaciones/check-ins equivalentes | una sola presencia efectiva | segundo intento idempotente o 409 |

</frozen-after-approval>

## Code Map

- `microservices/event-operations/internal/httpapi/server.go` -- ampliar deliveries para boleto/certificado y corregir `txCodeRequired`.
- `microservices/event-operations/internal/worker/hukid.go` -- cliente de emisión, OAuth, polling, errores y reconciliación.
- `microservices/onsite-operations/` -- añadir el adaptador de verificación y enlazar OID4VP con el check-in.
- `microservices/event-configuration/` -- origen de inscripción, evento y elegibilidad del boleto; no accede directamente a HukID.
- `frontend/src/microfrontends/onsite-operations/` -- iniciar/mostrar la sesión de presentación sin confiar en claims del navegador.
- `tools/hukid-simulator/` -- doble de desarrollo alineado con emisión y verificación.
- `docker-compose.yml` -- simulador local y conexión sustituible al nodo real.
- `/tmp/hukid-group5.JBL6nN/src/platform/contracts/issuance-v1.openapi.json` -- contrato de emisión del Grupo 5.
- `/tmp/hukid-group5.JBL6nN/src/platform/internal/verification/` -- referencia del flujo de verificación.

## Tasks & Acceptance

**Execution:**
- [x] `docs/contracts/hukid/` -- versionar la superficie externa de emisión/verificación consumida por Nexo.
- [x] `microservices/event-operations/` -- soportar boleto/certificado, OAuth cacheado, idempotencia y clasificación inicial de errores.
- [ ] `microservices/event-configuration/` -- publicar el hecho interno de inscripción confirmada que origina el boleto.
- [x] `microservices/onsite-operations/` -- crear sesiones OID4VP, persistir su contexto y registrar check-in al completar una sesión válida.
- [x] `tools/hukid-simulator/` -- reforzar validación del delivery y pruebas de replay/errores existentes.
- [ ] `frontend/` -- mostrar ofertas y QR de solicitud de presentación con ES/EN y sistema de diseño.
- [x] `docker-compose.yml`, migraciones y seeds -- configurar referencias HukID y schemas de contexto sin secretos reales.
- [ ] `tests/` y CI -- completar el E2E boleto → wallet simulada → presentación → check-in → certificado.

**Acceptance Criteria:**
- Given una inscripción confirmada, when Nexo procesa el evento, then solicita exactamente un boleto y expone una oferta OID4VCI válida.
- Given una wallet con boleto válido, when acepta la solicitud OID4VP, then HukID verifica la presentación y Nexo registra una sola presencia efectiva.
- Given una presentación auténtica de otro evento o scope, when Onsite la procesa, then rechaza el acceso sin crear asistencia.
- Given asistencia suficiente, when se solicita el certificado, then usa una definición distinta y reconcilia hasta `credential_issued`.
- Given un timeout o respuesta reintentable, when el worker reintenta, then conserva payload e `Idempotency-Key`; ante 422 o conflicto queda fallido sin segunda emisión.

## Implementation Notes

La referencia analizada corresponde a `juanrmos/hukid`, rama `develop`, commit `0001373`. El nodo permanece independiente y la wallet Flutter no se integra al monorepo. El flujo OID4VP estándar hace que la wallet escanee el QR mostrado por el verificador. El flujo inverso queda fuera hasta que HukID publique ese contrato.

## Spec Change Log

## Review Triage Log

- `medium` -- la primera implementación insertaba participaciones con `certificate_id = NULL`, pero el worker las descartaba al escanear a `string`; se corrigió usando `*string` y evitando actualizar certificados cuando la entrega es un boleto.
- `medium` -- la primera implementación no vinculaba las sesiones OID4VP con contexto Nexo ni check-in; se agregó persistencia local, `Idempotency-Key`, contexto de ticket/evento/scope y check-in al observar resultado `pass`.
- `medium` -- la primera prueba del cliente no verificaba paths ni TTL; se reforzó para validar rutas tenant/session, método, content type y cuerpo.
- `medium` -- la endpoint de boleto no verificaba una fuente de elegibilidad ni recibía un evento interno; queda pendiente conectar Event Configuration antes de habilitar emisión automática.
- `medium` -- el contrato del Grupo 5 devuelve resultado/digest, no claims del boleto; la validación exacta de ticket/holder depende de que la presentación de HukID use una definición/policy que restrinja `EventParticipationCredential`.

## Design Notes

```text
Registro/Eventos ──evento interno──> Adaptador emisión ──HTTPS──> Nodo HukID
Onsite ──sesión/verificación───────> Adaptador OID4VP ──HTTPS──> Nodo HukID
Wallet Flutter <──────────── OID4VCI / OID4VP ─────────> Nodo HukID
Onsite <──resultado verificado── adaptador ──> check-in transaccional
```

## Verification

**Commands:**
- `go test ./... && go vet ./...` en servicios Go afectados -- esperado: verde.
- `make contract-test` -- esperado: compatibilidad con contratos HukID.
- `docker compose config` -- esperado: configuración válida sin secretos reales.
- E2E del stack -- esperado: boleto emitido, presentación verificada, check-in único y certificado emitido.

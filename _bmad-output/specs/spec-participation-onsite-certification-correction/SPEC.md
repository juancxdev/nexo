# Especificación: corrección integral de registro, asistencia y certificación

Estado: propuesta lista para implementación  
Fecha: 2026-11-24  
Alcance: Event Configuration, Participation, Onsite Operations, Event Operations, frontend Nexo e integración HukID.

## 1. Problema

El flujo actual presenta cuatro fallos relacionados:

1. Un participante puede intentar registrarse varias veces con los mismos datos y el sistema no ofrece una respuesta de duplicidad clara, consistente e idempotente.
2. Operaciones presenciales usa identificadores y eventos de demostración hardcodeados, por lo que no está conectado de forma confiable con los eventos publicados y sus registros reales.
3. La vista de asistencia muestra hechos de check-in, no el padrón de participantes. Por ello una persona registrada no aparece hasta producir un hecho de asistencia.
4. La elegibilidad del certificado no valida de forma completa el cierre temporal del evento ni una regla versionada de asistencia. Esto puede habilitar una emisión prematura o inconsistente hacia HukID.

## 2. Resultado esperado

El flujo completo será:

```text
Evento publicado
      ↓
Formulario público habilitado
      ↓
Registro único e idempotente
      ↓
Credencial de entrada/ticket en HukID
      ↓
Participante visible en el padrón de Onsite
      ↓
Check-in / checkout / anulación append-only
      ↓
Evento finalizado + regla de asistencia cumplida
      ↓
Certificado elegible
      ↓
Emisión explícita de credencial-certificado a HukID
```

La credencial de entrada y la credencial-certificado son dos credenciales distintas. Registrarse no genera un certificado; hacer check-in tampoco lo emite automáticamente.

## 3. Capacidades requeridas

| ID | Capacidad | Resultado verificable |
|---|---|---|
| CAP-1 | Registro único | Solo existe un registro activo por formulario y correo normalizado; reintentos seguros no crean duplicados. |
| CAP-2 | Catálogo real de eventos | Participation y Onsite operan con eventos publicados del tenant activo, sin mapeos `demo` o `fiis`. |
| CAP-3 | Padrón operacional | Onsite lista todos los registros confirmados antes del check-in y muestra su estado efectivo. |
| CAP-4 | Asistencia consistente | Check-in, checkout y void son hechos inmutables; el estado visible se deriva transaccionalmente. |
| CAP-5 | Elegibilidad temporal | No puede emitirse certificado antes de finalizar el evento ni sin cumplir su regla publicada. |
| CAP-6 | Certificación HukID | Nexo solicita una credencial-certificado a HukID con evidencia congelada y consentimiento vigente. |
| CAP-7 | Seguridad transversal | Toda operación respeta tenant, scopes, auditoría, idempotencia y minimización de PII. |

## 4. Reglas no negociables

- Una base PostgreSQL por ambiente, separada por esquemas y roles.
- Ningún servicio lee tablas privadas de otro bounded context. Solo APIs, eventos o vistas contractuales propiedad del productor.
- No se crean foreign keys entre contextos.
- Todo identificador de evento proviene de datos persistidos; no de constantes del frontend.
- Toda escritura crítica usa `Idempotency-Key`.
- La asistencia es append-only.
- La emisión de certificado exige evento finalizado, regla cumplida, autorización y consentimiento.
- El navegador no construye claims HukID ni selecciona libremente issuer/credential definition.
- No se registran tokens, QR, claims completos, correos ni respuestas de formularios en logs.
- No se generará PDF de certificado en este incremento; la salida oficial será la credencial verificable de HukID.

## 5. Entregables

- [Flujos y estados](./workflows-and-state-models.md)
- [Contratos API y eventos](./api-and-events.md)
- [Modelo de datos y migraciones](./data-and-migrations.md)
- [Seguridad, aceptación y pruebas](./acceptance-and-tests.md)

## 6. Fuera de alcance

- Identidad nacional verificada como requisito obligatorio de inscripción.
- Operación offline de check-in.
- Implementación de la wallet o del nodo HukID.
- Generación de certificados PDF.
- Reglas académicas no definidas por el organizador.
- Fusión automática de dos registros históricos ambiguos.

## 7. Preguntas configurables, no bloqueantes

- La clave primaria de negocio inicial es el correo normalizado. En el futuro podrá añadirse un documento verificado cuando el formulario lo solicite.
- El modo de emisión será `manual` por defecto y podrá configurarse como `automatic` por evento.
- Eventos de varios días podrán exigir sesiones, días o porcentaje; la regla se configura y versiona, no se codifica de forma fija.

## 8. Orden recomendado de implementación

1. Corregir identidad e idempotencia de Participation.
2. Proyectar eventos publicados y eliminar hardcodes.
3. Construir el padrón operacional de Onsite.
4. Endurecer check-in, checkout, void y concurrencia.
5. Incorporar reglas versionadas y evaluación de elegibilidad.
6. Conectar emisión de certificado con HukID.
7. Completar frontend y E2E integral.


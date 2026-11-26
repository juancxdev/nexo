# Seguridad, aceptación y pruebas

## 1. Autorización

| Acción | Scope mínimo |
|---|---|
| Ver eventos y padrón | `onsite:read` |
| Check-in / checkout | `onsite:checkin` |
| Anular asistencia | `onsite:void` |
| Consultar elegibilidad | `reports:read` o `certificates:issue` |
| Emitir certificado | `certificates:issue` |
| Revocar/reemplazar | `certificates:revoke` |

Cada servicio valida firma JWT, `iss`, `aud`, `exp`, `tenant_id`, roles y scopes. `X-Mock-*` solo puede funcionar en `development/test` y el proceso debe negarse a iniciar en producción si está habilitado.

## 2. Controles de seguridad

- RLS o filtros obligatorios y testeados por tenant.
- Rate limit en formulario público, búsqueda y validación QR.
- Idempotency key con request hash para todas las escrituras.
- Cifrado de claims/ofertas HukID y secretos fuera del repositorio.
- Logs estructurados con trace ID y redacción de PII.
- Auditoría de publicación, check-in/out, void, regla, emisión, revocación y reintento.
- URLs/QR con tokens opacos o firmados, expiración y audience; nunca respuestas del formulario.
- Comparación constante para firmas/tokens sensibles cuando aplique.
- Límites de paginación y búsqueda para evitar extracción masiva del padrón.

## 3. Criterios de aceptación funcional

### AC-1 Registro único

```gherkin
Dado un formulario publicado y un correo " Apple.Juancx@Gmail.com "
Cuando se registra y luego se intenta registrar "apple.juancx@gmail.com"
Entonces existe un solo registro activo
Y el segundo intento devuelve 409 already_registered
Y no se genera un segundo ticket ni una segunda credencial HukID
```

### AC-2 Replay idempotente

```gherkin
Dada una solicitud aceptada con Idempotency-Key K
Cuando se repite K con el mismo payload
Entonces se reproduce la respuesta original
Y no se duplican registro, ticket, outbox ni emisión
Cuando se repite K con otro payload
Entonces se devuelve 409 idempotency_key_reused
```

### AC-3 Evento real en Onsite

```gherkin
Dado un evento publicado para IncubaUNAS
Cuando un operador abre Operaciones presenciales
Entonces el selector obtiene ese evento desde GET /onsite/v1/events
Y no depende de constantes demo/fiis
Y ningún evento de otro tenant es visible
```

### AC-4 Padrón antes del check-in

```gherkin
Dado un participante confirmado sin asistencia
Cuando se consulta el padrón del evento
Entonces aparece con estado confirmed
Y el resumen lo cuenta como confirmado, no como presente
```

### AC-5 Transición de asistencia

```gherkin
Dado un participante confirmed
Cuando se registra un check-in válido
Entonces su estado cambia a present y se guarda la hora
Cuando se registra checkout
Entonces cambia a checked_out
Cuando un supervisor anula el hecho aplicable
Entonces el estado se recalcula desde los hechos restantes
```

### AC-6 Concurrencia

```gherkin
Dado un ticket válido
Cuando dos operadores ejecutan check-in simultáneamente
Entonces solo una operación crea presencia efectiva
Y la otra recibe replay o 409 already_present
```

### AC-7 Bloqueo temporal del certificado

```gherkin
Dado un participante presente y un evento aún no finalizado
Cuando se evalúa elegibilidad
Entonces eligible es false con event_not_finished
Y la API rechaza la emisión
```

### AC-8 Certificado elegible y HukID

```gherkin
Dado un evento finalizado, registro confirmado, check-in efectivo y consentimiento vigente
Cuando un usuario con certificates:issue solicita la emisión
Entonces se congela evidencia y se crea un solo certificado
Y se solicita una credencial-certificado a HukID de forma idempotente
Y la wallet puede consentir y recibirla
```

### AC-9 Separación de credenciales

```gherkin
Dado un registro recién confirmado
Entonces su credencial de entrada puede existir
Pero no existe credencial-certificado hasta cumplir elegibilidad y solicitar emisión
```

## 4. Matriz de pruebas

### Unitarias

- normalización de correo;
- hash canónico e idempotencia;
- máquina de estados de roster;
- reducción de facts a estado efectivo;
- reglas temporales y por sesión;
- mapeo de estados HukID.

### Integración con PostgreSQL real

- índice único con mayúsculas y espacios;
- carrera de dos registros concurrentes;
- carrera de check-ins;
- outbox e inbox con replay;
- aislamiento tenant A/B;
- migración de datos existentes;
- reconstrucción del roster.

### Contrato

- OpenAPI de Participation, Onsite y Operations;
- JSON Schema de todos los eventos;
- compatibilidad del adaptador con HukID;
- errores RFC 9457;
- validación de que no se exponen campos prohibidos.

### E2E obligatorio

1. Crear usuario y tenant.
2. Crear y publicar evento, cronograma, actividad y formulario.
3. Abrir URL pública y registrar participante.
4. Ver ticket/credencial de entrada disponible.
5. Repetir registro con correo equivalente y comprobar bloqueo.
6. Iniciar sesión como operador y seleccionar el mismo evento real.
7. Ver al participante en padrón con estado confirmado.
8. Buscar ticket manualmente y escanear QR HukID.
9. Hacer check-in y comprobar estado presente/resumen.
10. Intentar certificado antes del fin y comprobar bloqueo.
11. Usar evento finalizado del seed o avanzar reloj de test.
12. Emitir credencial-certificado y aceptar oferta en wallet/simulador HukID.
13. Comprobar estado `credential_issued`.
14. Cambiar tenant/idioma y verificar aislamiento e i18n.

## 5. Pruebas negativas HukID

- `401`: renovar token una vez.
- `409 idempotency_key_reused`: error crítico sin generar otra key.
- `422`: fallo definitivo de contrato/claims.
- `429`: respetar `Retry-After`.
- `503`/timeout: backoff con jitter.
- oferta expirada: marcar expirada y permitir reintento controlado.
- replay concurrente: una sola solicitud/credencial efectiva.

## 6. Rendimiento y observabilidad

- check-in online p95 menor de 700 ms bajo carga objetivo;
- listado de roster paginado p95 menor de 1 s para 10 000 registros;
- tres veces el pico estimado sin duplicar presencia;
- métricas: duplicados bloqueados, lag de proyección, eventos sin mapear, check-ins por resultado, elegibilidad por razón, deliveries HukID por estado;
- alertas por lag de proyección, errores de outbox, discrepancia de conteos y tasa elevada de fallos HukID.

## 7. Definition of Done

- Migraciones y backfill probados sobre una copia sanitizada.
- OpenAPI y eventos versionados y validados en CI.
- No quedan hardcodes de eventos en frontend ni backend.
- Padrón real visible antes del check-in.
- Pruebas de duplicidad, concurrencia, tenant y tiempo en verde.
- E2E completo ejecutado con PostgreSQL, Core, Event Configuration, Participation, Onsite, Event Operations y HukID simulator/nodo.
- Auditoría de seguridad sin hallazgos críticos o altos abiertos.
- Documentación operativa de reconciliación y rollback disponible.

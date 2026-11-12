# HukID Integration API Simulator

Servidor local compatible con la superficie privada de HukID que consume Nexo:

- `POST /oauth2/token`
- `POST /v1/issuance-requests`
- `GET /v1/issuance-requests/{id}`
- `POST /v1/issuance-requests/{id}/cancellations`

Las rutas `/__test` solo sirven para pruebas y simulan el canje de una wallet:

```bash
curl -X POST http://localhost:8090/__test/issuance-requests/IR_ID/issue
```

También permite forzar errores con `POST /__test/faults` y reiniciar el estado
con `POST /__test/reset`. No representa el servidor productivo de HukID.

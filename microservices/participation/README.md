# Participation API

Servicio público de formularios, registros y tickets de participación. Usa el esquema `participation` dentro de la base PostgreSQL central `nexo`; no levanta una base propia.

## Desarrollo

```bash
docker compose -f docker-compose.yml up -d postgres
docker compose -f docker-compose.init.yml run --rm migrate-participation
docker compose -f docker-compose.init.yml run --rm seed-participation
docker compose up -d participation-api
```

El ticket de desarrollo es un JWS de fixture. La validación criptográfica definitiva se conectará al emisor de Participación/Onsite cuando publique su `JWKS`.

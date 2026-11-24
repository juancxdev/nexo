ROOT := ../..
COMPOSE := docker compose -f $(ROOT)/docker-compose.yml
COMPOSE_INIT := docker compose -f $(ROOT)/docker-compose.yml -f $(ROOT)/docker-compose.init.yml
DATABASE_URL ?= postgres://nexo:nexo-local-only@localhost:5433/nexo?sslmode=disable

.PHONY: db-up db-migrate db-seed db-reset run compose-up compose-down test vet

db-up:
	$(COMPOSE) up -d postgres
db-migrate:
	$(COMPOSE_INIT) run --rm --no-deps migrate-core
	$(COMPOSE_INIT) run --rm --no-deps migrate-onsite
db-seed:
	$(COMPOSE_INIT) run --rm --no-deps seed-onsite
db-reset:
	$(COMPOSE) down -v
	$(MAKE) db-up db-migrate db-seed
run:
	DATABASE_URL="$(DATABASE_URL)" APP_ENV=development go run ./cmd/api
compose-up:
	$(COMPOSE) up -d
compose-down:
	$(COMPOSE) down
test:
	$(MAKE) db-up db-migrate db-seed
	ONSITE_TEST_DATABASE_URL="$(DATABASE_URL)" APP_ENV=test NEXO_TEST_MODE=true go test ./...
vet:
	go vet ./...

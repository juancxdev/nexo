.PHONY: stack-init deploy-core deploy-onsite deploy-operations deploy-events deploy-hukid events-up events-rebuild events-logs events-down

stack-init:
	./scripts/dev-up.sh

deploy-core:
	./scripts/deploy-service.sh core

deploy-onsite:
	./scripts/deploy-service.sh onsite

deploy-operations:
	./scripts/deploy-service.sh operations

deploy-events:
	./scripts/deploy-service.sh events

deploy-hukid:
	./scripts/deploy-service.sh hukid

events-up:
	./scripts/deploy-service.sh events

events-rebuild:
	./scripts/deploy-service.sh events

events-logs:
	docker compose logs -f event-configuration-api event-configuration-worker

events-down:
	docker compose stop event-configuration-api event-configuration-worker

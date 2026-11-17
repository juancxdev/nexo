.PHONY: events-up events-rebuild events-logs events-down

events-up:
	docker compose up -d postgres bootstrap-events migrate-events seed-events event-configuration-api event-configuration-worker

events-rebuild:
	docker compose up -d --build event-configuration-api event-configuration-worker

events-logs:
	docker compose logs -f event-configuration-api event-configuration-worker

events-down:
	docker compose stop event-configuration-api event-configuration-worker

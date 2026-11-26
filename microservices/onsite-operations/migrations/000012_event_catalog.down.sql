DROP INDEX IF EXISTS onsite.onsite_events_tenant_status_idx;
ALTER TABLE onsite.events DROP COLUMN IF EXISTS participation_event_id;
ALTER TABLE onsite.events DROP COLUMN IF EXISTS published_revision;
ALTER TABLE onsite.events DROP COLUMN IF EXISTS timezone;

ALTER TABLE onsite.events
  ADD COLUMN IF NOT EXISTS timezone text NOT NULL DEFAULT 'America/Lima',
  ADD COLUMN IF NOT EXISTS published_revision integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS participation_event_id text;

CREATE INDEX IF NOT EXISTS onsite_events_tenant_status_idx ON onsite.events(tenant_id,status,starts_at);

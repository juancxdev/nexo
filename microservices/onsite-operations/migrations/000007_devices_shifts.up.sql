ALTER TABLE onsite.devices
  ADD COLUMN IF NOT EXISTS venue_id text NOT NULL DEFAULT 'main',
  ADD COLUMN IF NOT EXISTS public_key text,
  ADD COLUMN IF NOT EXISTS release_version text NOT NULL DEFAULT 'dev',
  ADD COLUMN IF NOT EXISTS last_seen_at timestamptz;

CREATE TABLE IF NOT EXISTS onsite.shifts(
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  event_id uuid NOT NULL REFERENCES onsite.events(id),
  scope_id uuid NOT NULL REFERENCES onsite.scopes(id),
  device_id uuid NOT NULL REFERENCES onsite.devices(id),
  actor_id uuid NOT NULL,
  status text NOT NULL CHECK(status IN ('open','closed')),
  opened_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz,
  UNIQUE(event_id, scope_id, device_id, actor_id, status)
);
CREATE INDEX IF NOT EXISTS onsite_shifts_active_idx ON onsite.shifts(tenant_id,event_id,scope_id) WHERE status='open';

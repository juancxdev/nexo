ALTER TABLE participation.forms
  ADD COLUMN IF NOT EXISTS tenant_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001';

ALTER TABLE participation.registrations
  ADD COLUMN IF NOT EXISTS hukid_delivery_id uuid,
  ADD COLUMN IF NOT EXISTS hukid_state text NOT NULL DEFAULT 'not_requested';

CREATE INDEX IF NOT EXISTS participation_registrations_hukid_delivery
  ON participation.registrations(hukid_delivery_id);

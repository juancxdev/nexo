ALTER TABLE integration.hukid_configs
  ADD COLUMN IF NOT EXISTS participation_credential_definition text;

ALTER TABLE integration.hukid_deliveries
  ALTER COLUMN certificate_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS credential_type text NOT NULL DEFAULT 'attendance_certificate',
  ADD COLUMN IF NOT EXISTS subject_reference text,
  ADD COLUMN IF NOT EXISTS event_id text,
  ADD COLUMN IF NOT EXISTS registration_id text;

ALTER TABLE integration.hukid_deliveries
  DROP CONSTRAINT IF EXISTS hukid_deliveries_credential_type_check;
ALTER TABLE integration.hukid_deliveries
  ADD CONSTRAINT hukid_deliveries_credential_type_check
  CHECK (credential_type IN ('participation_ticket', 'attendance_certificate'));

CREATE UNIQUE INDEX IF NOT EXISTS hukid_deliveries_subject_idx
  ON integration.hukid_deliveries(tenant_id, credential_type, subject_reference)
  WHERE subject_reference IS NOT NULL;

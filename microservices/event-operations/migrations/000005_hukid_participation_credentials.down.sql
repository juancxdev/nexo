DROP INDEX IF EXISTS integration.hukid_deliveries_subject_idx;
ALTER TABLE integration.hukid_deliveries DROP CONSTRAINT IF EXISTS hukid_deliveries_credential_type_check;
ALTER TABLE integration.hukid_deliveries
  DROP COLUMN IF EXISTS credential_type,
  DROP COLUMN IF EXISTS subject_reference,
  DROP COLUMN IF EXISTS event_id,
  DROP COLUMN IF EXISTS registration_id;
ALTER TABLE integration.hukid_configs DROP COLUMN IF EXISTS participation_credential_definition;

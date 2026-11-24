DROP INDEX IF EXISTS participation_registrations_hukid_delivery;
ALTER TABLE participation.registrations
  DROP COLUMN IF EXISTS hukid_delivery_id,
  DROP COLUMN IF EXISTS hukid_state;
ALTER TABLE participation.forms DROP COLUMN IF EXISTS tenant_id;

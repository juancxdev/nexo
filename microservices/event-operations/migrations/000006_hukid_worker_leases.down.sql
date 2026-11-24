DROP INDEX IF EXISTS hukid_deliveries_due_idx;
ALTER TABLE integration.hukid_deliveries DROP COLUMN IF EXISTS lease_until;

ALTER TABLE integration.hukid_deliveries
  ADD COLUMN IF NOT EXISTS lease_until timestamptz;

CREATE INDEX IF NOT EXISTS hukid_deliveries_due_idx
  ON integration.hukid_deliveries(state, next_attempt_at);

ALTER TABLE participation.registrations
  ADD COLUMN IF NOT EXISTS email_normalized text,
  ADD COLUMN IF NOT EXISTS participant_fingerprint text;

UPDATE participation.registrations
SET email_normalized = lower(trim(email))
WHERE email_normalized IS NULL;

ALTER TABLE participation.registrations
  ALTER COLUMN email_normalized SET NOT NULL;

DROP INDEX IF EXISTS participation.registrations_form_id_email_key;
CREATE UNIQUE INDEX IF NOT EXISTS registrations_unique_email_per_form
  ON participation.registrations(form_id, email_normalized)
  WHERE state IN ('pending','confirmed','waitlisted');

CREATE INDEX IF NOT EXISTS registrations_fingerprint_idx
  ON participation.registrations(form_id, participant_fingerprint)
  WHERE participant_fingerprint IS NOT NULL;

CREATE OR REPLACE VIEW participation_contract.registrations_v1 WITH (security_barrier) AS
SELECT r.id AS registration_id, f.tenant_id, f.event_id, f.site_slug, r.ticket_id,
       r.full_name, r.state, r.ticket_state
FROM participation.registrations r
JOIN participation.forms f ON f.id = r.form_id
WHERE r.state='confirmed';

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='operations_app') THEN
    GRANT USAGE ON SCHEMA participation_contract TO operations_app;
    GRANT SELECT ON ALL TABLES IN SCHEMA participation_contract TO operations_app;
  END IF;
END $$;

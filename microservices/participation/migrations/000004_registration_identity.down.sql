DROP INDEX IF EXISTS participation.registrations_fingerprint_idx;
DROP INDEX IF EXISTS participation.registrations_unique_email_per_form;
ALTER TABLE participation.registrations DROP COLUMN IF EXISTS participant_fingerprint;
ALTER TABLE participation.registrations DROP COLUMN IF EXISTS email_normalized;

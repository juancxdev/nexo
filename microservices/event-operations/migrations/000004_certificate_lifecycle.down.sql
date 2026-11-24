DROP INDEX IF EXISTS certificates.certificates_one_active;
ALTER TABLE certificates.certificates DROP COLUMN IF EXISTS ticket_id, DROP COLUMN IF EXISTS replaced_by,
  DROP COLUMN IF EXISTS replaces_id, DROP COLUMN IF EXISTS revocation_reason;

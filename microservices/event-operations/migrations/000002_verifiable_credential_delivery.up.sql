-- Certificates are evidence records. The verifiable credential is issued by
-- HukID, therefore Nexo neither produces a PDF nor stores a document artifact.
ALTER TABLE certificates.certificates
  ALTER COLUMN artifact_key DROP NOT NULL,
  ALTER COLUMN artifact_sha256 DROP NOT NULL;

ALTER TABLE certificates.certificates DROP CONSTRAINT IF EXISTS certificates_certificates_state_check;
ALTER TABLE certificates.certificates DROP CONSTRAINT IF EXISTS certificates_state_check;
UPDATE certificates.certificates
SET state = 'credential_issued', artifact_key = NULL
WHERE state = 'issued';
ALTER TABLE certificates.certificates
  ADD CONSTRAINT certificates_certificates_state_check CHECK (state IN (
    'eligible','request_pending','offer_available','offer_redeemed',
    'credential_issued','failed','cancelled','revoked','replaced'
  ));

ALTER TABLE certificates.certificates
  ADD COLUMN IF NOT EXISTS evidence_digest text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS eligibility_rule_version text NOT NULL DEFAULT 'attendance@1.0.0',
  ADD COLUMN IF NOT EXISTS claims_hash text,
  ADD COLUMN IF NOT EXISTS consent_granted_at timestamptz;

ALTER TABLE integration.hukid_deliveries
  ADD COLUMN IF NOT EXISTS claims_ciphertext bytea,
  ADD COLUMN IF NOT EXISTS claims_nonce bytea,
  ADD COLUMN IF NOT EXISTS claims_key_version text,
  ADD COLUMN IF NOT EXISTS claims_hash text,
  ADD COLUMN IF NOT EXISTS consent_granted_at timestamptz,
  ADD COLUMN IF NOT EXISTS offer_expires_at timestamptz,
  ADD COLUMN IF NOT EXISTS lease_until timestamptz;

CREATE INDEX IF NOT EXISTS hukid_deliveries_due_idx
  ON integration.hukid_deliveries(state, next_attempt_at);

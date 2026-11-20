DROP INDEX IF EXISTS integration.hukid_deliveries_due_idx;
ALTER TABLE integration.hukid_deliveries
  DROP COLUMN IF EXISTS offer_expires_at,
  DROP COLUMN IF EXISTS consent_granted_at,
  DROP COLUMN IF EXISTS claims_hash,
  DROP COLUMN IF EXISTS claims_key_version,
  DROP COLUMN IF EXISTS claims_nonce,
  DROP COLUMN IF EXISTS claims_ciphertext;
ALTER TABLE certificates.certificates
  DROP COLUMN IF EXISTS consent_granted_at,
  DROP COLUMN IF EXISTS claims_hash,
  DROP COLUMN IF EXISTS eligibility_rule_version,
  DROP COLUMN IF EXISTS evidence_digest;
ALTER TABLE integration.hukid_deliveries DROP COLUMN IF EXISTS lease_until;

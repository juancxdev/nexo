-- Ciclo de vida del certificado (OO-024/025): revocación con motivo y reemisión enlazada.
ALTER TABLE certificates.certificates
  ADD COLUMN IF NOT EXISTS revocation_reason text,
  ADD COLUMN IF NOT EXISTS replaces_id uuid REFERENCES certificates.certificates(id),
  ADD COLUMN IF NOT EXISTS replaced_by uuid REFERENCES certificates.certificates(id),
  ADD COLUMN IF NOT EXISTS ticket_id text;
-- Un participante tiene a lo sumo un certificado vigente por evento.
CREATE UNIQUE INDEX IF NOT EXISTS certificates_one_active
  ON certificates.certificates(tenant_id, event_id, registration_id)
  WHERE state NOT IN ('revoked','replaced','cancelled','failed');

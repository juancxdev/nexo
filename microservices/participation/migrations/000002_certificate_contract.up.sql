-- Contrato de lectura para Event Operations: lo mínimo para decidir y nombrar un
-- certificado (OO-024). Sólo registros confirmados; sin correo, respuestas ni tokens.
CREATE SCHEMA IF NOT EXISTS participation_contract;
CREATE OR REPLACE VIEW participation_contract.registrations_v1 WITH (security_barrier) AS
SELECT r.id AS registration_id, f.event_id, f.site_slug, r.ticket_id, r.full_name, r.state, r.ticket_state
FROM participation.registrations r JOIN participation.forms f ON f.id=r.form_id
WHERE r.state='confirmed';
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='operations_app') THEN
    GRANT USAGE ON SCHEMA participation_contract TO operations_app;
    GRANT SELECT ON ALL TABLES IN SCHEMA participation_contract TO operations_app;
  END IF;
END $$;

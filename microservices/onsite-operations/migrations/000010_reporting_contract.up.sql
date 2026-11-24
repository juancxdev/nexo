-- Contrato de lectura para Event Operations (reportes y elegibilidad de certificados).
-- Event Operations NO lee las tablas de Onsite: sólo estas vistas versionadas, de solo
-- lectura y sin datos personales (el titular va enmascarado).
CREATE SCHEMA IF NOT EXISTS onsite_contract;

CREATE OR REPLACE VIEW onsite_contract.events_v1 WITH (security_barrier) AS
SELECT e.tenant_id, e.source_event_id AS event_id, e.participation_event_id, e.name, e.starts_at, e.ends_at
FROM onsite.events e;

-- Cada hecho de entrada o salida con su estado de anulación.
CREATE OR REPLACE VIEW onsite_contract.attendance_v1 WITH (security_barrier) AS
SELECT e.tenant_id, e.source_event_id AS event_id, sc.code AS scope, f.id AS fact_id, f.ticket_id, f.type,
       f.occurred_at, EXISTS(SELECT 1 FROM onsite.attendance_facts v WHERE v.type='void' AND v.compensates_fact_id=f.id) AS voided,
       COALESCE(t.masked_holder,'') AS masked_holder
FROM onsite.attendance_facts f
JOIN onsite.events e ON e.id=f.event_id
JOIN onsite.scopes sc ON sc.id=f.scope_id
LEFT JOIN onsite.ticket_projections t ON t.ticket_id=f.ticket_id
WHERE f.type IN ('check_in','check_out');

-- Quién está dentro ahora.
CREATE OR REPLACE VIEW onsite_contract.presence_v1 WITH (security_barrier) AS
SELECT e.tenant_id, e.source_event_id AS event_id, sc.code AS scope, p.ticket_id, p.checked_in_at,
       COALESCE(t.masked_holder,'') AS masked_holder
FROM onsite.effective_attendance p
JOIN onsite.events e ON e.id=p.event_id
JOIN onsite.scopes sc ON sc.id=p.scope_id
LEFT JOIN onsite.ticket_projections t ON t.ticket_id=p.ticket_id;

-- Rechazos de escaneo por motivo (sin QR ni datos personales).
CREATE OR REPLACE VIEW onsite_contract.scan_rejections_v1 WITH (security_barrier) AS
SELECT r.tenant_id, e.source_event_id AS event_id, r.scope_code AS scope, r.reason, r.occurred_at
FROM onsite.scan_rejections r JOIN onsite.events e ON e.id=r.event_id;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='operations_app') THEN
    GRANT USAGE ON SCHEMA onsite_contract TO operations_app;
    GRANT SELECT ON ALL TABLES IN SCHEMA onsite_contract TO operations_app;
  END IF;
END $$;

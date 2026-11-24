-- Reportes reales (OO-020/021): tipo de reporte, archivo generado y su ciclo de vida.
-- El archivo se guarda en la base (no hay almacenamiento de objetos en el plan gratuito)
-- y se purga al vencer.
ALTER TABLE operations.report_jobs
  ADD COLUMN IF NOT EXISTS report_type text NOT NULL DEFAULT 'attendance',
  ADD COLUMN IF NOT EXISTS artifact bytea,
  ADD COLUMN IF NOT EXISTS content_type text,
  ADD COLUMN IF NOT EXISTS row_count int,
  ADD COLUMN IF NOT EXISTS attempts int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS lease_until timestamptz;
ALTER TABLE operations.report_jobs DROP CONSTRAINT IF EXISTS report_jobs_type_check;
ALTER TABLE operations.report_jobs ADD CONSTRAINT report_jobs_type_check
  CHECK (report_type IN ('attendance','presence','rejections','summary'));
CREATE INDEX IF NOT EXISTS report_jobs_due_idx ON operations.report_jobs(status, created_at);
CREATE INDEX IF NOT EXISTS report_jobs_event_idx ON operations.report_jobs(tenant_id, event_id, created_at DESC);

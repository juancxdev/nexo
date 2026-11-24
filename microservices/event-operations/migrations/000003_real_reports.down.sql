DROP INDEX IF EXISTS operations.report_jobs_event_idx;
DROP INDEX IF EXISTS operations.report_jobs_due_idx;
ALTER TABLE operations.report_jobs DROP CONSTRAINT IF EXISTS report_jobs_type_check;
ALTER TABLE operations.report_jobs DROP COLUMN IF EXISTS lease_until, DROP COLUMN IF EXISTS attempts,
  DROP COLUMN IF EXISTS row_count, DROP COLUMN IF EXISTS content_type, DROP COLUMN IF EXISTS artifact,
  DROP COLUMN IF EXISTS report_type;

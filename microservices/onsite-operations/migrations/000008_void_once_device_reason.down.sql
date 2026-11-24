ALTER TABLE onsite.devices DROP COLUMN IF EXISTS revoked_reason;
DROP INDEX IF EXISTS onsite.attendance_single_void;

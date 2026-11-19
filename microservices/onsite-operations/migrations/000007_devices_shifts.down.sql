DROP TABLE IF EXISTS onsite.shifts;
ALTER TABLE onsite.devices DROP COLUMN IF EXISTS last_seen_at, DROP COLUMN IF EXISTS release_version, DROP COLUMN IF EXISTS public_key, DROP COLUMN IF EXISTS venue_id;

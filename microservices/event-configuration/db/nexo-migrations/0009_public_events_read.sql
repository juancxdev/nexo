-- 0009_public_events_read.sql
-- Forward-only migration: exposes a strictly scoped read-only function for
-- publicly listed published events without leaking organization_id or drafts.

CREATE OR REPLACE FUNCTION events.list_public_published_events(p_limit integer DEFAULT 20)
RETURNS TABLE (
  id uuid,
  slug varchar,
  title varchar,
  description text,
  kind varchar,
  starts_at timestamptz,
  ends_at timestamptz,
  timezone varchar
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = events, pg_temp
AS $$
  SELECT
    e.id,
    e.slug,
    e.title,
    e.description,
    e.kind,
    e.starts_at,
    e.ends_at,
    e.timezone
  FROM events.events e
  WHERE e.status = 'published'
  ORDER BY e.starts_at ASC, e.id ASC
  LIMIT GREATEST(1, LEAST(COALESCE(p_limit, 20), 50));
$$;

REVOKE ALL ON FUNCTION events.list_public_published_events(integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION events.list_public_published_events(integer) TO configuration_app;

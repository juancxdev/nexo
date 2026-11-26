-- 0010_public_events_rls_policy.sql
-- Forward-only migration: allow events.list_public_published_events() to read
-- published rows under FORCE ROW LEVEL SECURITY via a scoped session setting.

DROP POLICY IF EXISTS events_public_published_read ON events.events;
CREATE POLICY events_public_published_read ON events.events
  FOR SELECT
  USING (
    status = 'published'
    AND current_setting('app.public_events_read', true) = 'on'
  );

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
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = events, pg_temp
AS $$
BEGIN
  PERFORM set_config('app.public_events_read', 'on', true);
  RETURN QUERY
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
END;
$$;

REVOKE ALL ON FUNCTION events.list_public_published_events(integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION events.list_public_published_events(integer) TO configuration_app;

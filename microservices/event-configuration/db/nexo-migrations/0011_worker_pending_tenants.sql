-- 0011_worker_pending_tenants.sql
-- Forward-only migration: allow configuration_worker to discover organizations
-- with pending outbox messages without exposing message payloads across tenants.

DROP POLICY IF EXISTS outbox_pending_tenant_discovery ON events.outbox;
CREATE POLICY outbox_pending_tenant_discovery ON events.outbox
  FOR SELECT
  USING (
    status = 'pending'
    AND current_setting('app.outbox_tenant_discovery', true) = 'on'
  );

CREATE OR REPLACE FUNCTION events.list_pending_outbox_organizations()
RETURNS TABLE (organization_id uuid)
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = events, pg_temp
AS $$
BEGIN
  PERFORM set_config('app.outbox_tenant_discovery', 'on', true);
  RETURN QUERY
  SELECT DISTINCT o.organization_id
  FROM events.outbox o
  WHERE o.status = 'pending'
    AND o.next_retry_at <= clock_timestamp()
    AND (o.locked_until IS NULL OR o.locked_until < clock_timestamp());
END;
$$;

REVOKE ALL ON FUNCTION events.list_pending_outbox_organizations() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION events.list_pending_outbox_organizations() TO configuration_app;

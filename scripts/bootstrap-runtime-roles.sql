-- Runtime roles for the single central database.
-- This file is executed only by the local/CI bootstrap job as the migrator role.
-- Production credentials must come from a secret manager, not this repository.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'core_app') THEN
    CREATE ROLE core_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD 'core-app-local-only';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'onsite_app') THEN
    CREATE ROLE onsite_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD 'onsite-app-local-only';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'operations_app') THEN
    CREATE ROLE operations_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD 'operations-app-local-only';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'participation_app') THEN
    CREATE ROLE participation_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD 'participation-app-local-only';
  END IF;
END $$;

ALTER ROLE core_app NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE onsite_app NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE operations_app NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE participation_app NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;

GRANT CONNECT ON DATABASE nexo TO core_app, onsite_app, operations_app, participation_app;

GRANT USAGE ON SCHEMA participation TO participation_app;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA participation TO participation_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA participation TO participation_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA participation GRANT SELECT, INSERT, UPDATE ON TABLES TO participation_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA participation GRANT USAGE, SELECT ON SEQUENCES TO participation_app;

GRANT USAGE ON SCHEMA identity TO core_app;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA identity TO core_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA identity TO core_app;

-- Core needs to resolve identities and memberships, while the tables keep RLS
-- enabled. Runtime access is limited to this role and this schema.
DROP POLICY IF EXISTS core_app_users ON identity.users;
CREATE POLICY core_app_users ON identity.users FOR ALL TO core_app USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS core_app_tenants ON identity.tenants;
CREATE POLICY core_app_tenants ON identity.tenants FOR ALL TO core_app USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS core_app_memberships ON identity.memberships;
CREATE POLICY core_app_memberships ON identity.memberships FOR ALL TO core_app USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS core_app_sessions ON identity.sessions;
CREATE POLICY core_app_sessions ON identity.sessions FOR ALL TO core_app USING (true) WITH CHECK (true);

GRANT USAGE ON SCHEMA onsite TO onsite_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA onsite TO onsite_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA onsite TO onsite_app;

GRANT USAGE ON SCHEMA operations, certificates, integration TO operations_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA operations, certificates, integration TO operations_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA operations, certificates, integration TO operations_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA identity GRANT SELECT, INSERT, UPDATE ON TABLES TO core_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA identity GRANT USAGE, SELECT ON SEQUENCES TO core_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA onsite GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO onsite_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA onsite GRANT USAGE, SELECT ON SEQUENCES TO onsite_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA operations, certificates, integration GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO operations_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA operations, certificates, integration GRANT USAGE, SELECT ON SEQUENCES TO operations_app;

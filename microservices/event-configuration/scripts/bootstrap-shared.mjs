import { createRequire } from 'node:module';

const requireFromApi = createRequire(new URL('../apps/configuration-api/package.json', import.meta.url));
const { Client } = requireFromApi('pg');

if (process.env.NEXO_SHARED_DATABASE !== 'true') {
  throw new Error('NEXO_SHARED_DATABASE=true is required for the central bootstrap.');
}

const database = process.env.POSTGRES_DB;
const host = process.env.POSTGRES_HOST;
const admin = process.env.POSTGRES_USER;
const adminPassword = process.env.POSTGRES_PASSWORD;
const migrator = process.env.EVENTS_MIGRATOR_USER || 'events_migrator';
const migratorPassword = process.env.EVENTS_MIGRATOR_PASSWORD;
const runtime = process.env.APP_DB_USER || 'configuration_app';
const runtimePassword = process.env.APP_DB_PASSWORD;

if (database !== 'nexo' || !host || !admin || !adminPassword || !migratorPassword || !runtimePassword) {
  throw new Error('Central bootstrap requires nexo database and all PostgreSQL credentials.');
}
for (const role of [admin, migrator, runtime]) {
  if (!/^[a-z_][a-z0-9_]*$/.test(role)) throw new Error('Invalid PostgreSQL role name.');
}
if (new Set([admin, migrator, runtime]).size !== 3) {
  throw new Error('Platform admin, events migrator and runtime roles must be distinct.');
}

const ident = (value) => `"${value}"`;
const literal = (value) => `'${value.replaceAll("'", "''")}'`;
const client = new Client({ host, port: Number(process.env.POSTGRES_PORT || 5432), database, user: admin, password: adminPassword });

try {
  await client.connect();
  await client.query('CREATE EXTENSION IF NOT EXISTS pgcrypto');
  await client.query('CREATE EXTENSION IF NOT EXISTS btree_gist');

  for (const [role, password] of [[migrator, migratorPassword], [runtime, runtimePassword]]) {
    const exists = await client.query('SELECT 1 FROM pg_roles WHERE rolname = $1', [role]);
    const command = exists.rowCount ? 'ALTER ROLE' : 'CREATE ROLE';
    await client.query(`${command} ${ident(role)} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD ${literal(password)}`);
  }

  const inheritedRoles = await client.query(
    'SELECT pg_get_userbyid(roleid) AS role FROM pg_auth_members WHERE member = (SELECT oid FROM pg_roles WHERE rolname = $1)',
    [runtime],
  );
  for (const membership of inheritedRoles.rows) {
    await client.query(`REVOKE ${ident(membership.role)} FROM ${ident(runtime)}`);
  }
  await client.query(`REVOKE ALL PRIVILEGES ON DATABASE ${ident(database)} FROM ${ident(runtime)}`);
  await client.query(`GRANT CONNECT ON DATABASE ${ident(database)} TO ${ident(runtime)}`);
  await client.query(`REVOKE ALL PRIVILEGES ON SCHEMA public FROM ${ident(runtime)}`);
  await client.query(`REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM ${ident(runtime)}`);
  await client.query(`REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM ${ident(runtime)}`);

  await client.query('CREATE SCHEMA IF NOT EXISTS platform_meta');
  await client.query(`CREATE SCHEMA IF NOT EXISTS events AUTHORIZATION ${ident(migrator)}`);
  const owner = await client.query("SELECT pg_get_userbyid(nspowner) AS owner FROM pg_namespace WHERE nspname = 'events'");
  if (owner.rows[0]?.owner !== migrator) throw new Error('The events schema has an unexpected owner.');

  await client.query('CREATE TABLE IF NOT EXISTS platform_meta.events_schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)');
  await client.query('CREATE TABLE IF NOT EXISTS platform_meta.events_migration_checksums (version bigint PRIMARY KEY, filename text UNIQUE NOT NULL, sha256 text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())');
  await client.query(`ALTER TABLE platform_meta.events_schema_migrations OWNER TO ${ident(migrator)}`);
  await client.query(`ALTER TABLE platform_meta.events_migration_checksums OWNER TO ${ident(migrator)}`);
  await client.query(`GRANT USAGE ON SCHEMA events TO ${ident(runtime)}`);
  await client.query(`GRANT USAGE ON SCHEMA platform_meta TO ${ident(migrator)}`);
  console.log('Bootstrap central de Event Configuration completado.');
} finally {
  await client.end();
}

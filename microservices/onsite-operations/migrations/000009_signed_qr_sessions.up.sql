-- OO-010/OO-016/OO-017: validación local del QR firmado por Participation y registro de rechazos.
-- OO-015: control de acceso por sesión sin alterar la asistencia principal.

-- Claves públicas Ed25519 de Participation, por tenant. Nunca se guarda una clave privada.
CREATE TABLE IF NOT EXISTS onsite.ticket_keys(
  kid text PRIMARY KEY,
  tenant_id uuid NOT NULL,
  alg text NOT NULL CHECK (alg = 'EdDSA'),
  public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
  status text NOT NULL CHECK (status IN ('active','retired','compromised')),
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Revocaciones sincronizadas y su frescura (para aceptar tickets provisionales).
CREATE TABLE IF NOT EXISTS onsite.ticket_revocations(
  tenant_id uuid NOT NULL,
  jti text NOT NULL,
  revoked_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, jti)
);
CREATE TABLE IF NOT EXISTS onsite.revocation_state(
  tenant_id uuid PRIMARY KEY,
  list_version bigint NOT NULL,
  synced_at timestamptz NOT NULL
);

-- El evento de Onsite conoce el ID con que Participation emite el QR (p. ej. demo-day-2026).
ALTER TABLE onsite.events ADD COLUMN IF NOT EXISTS participation_event_id text;
CREATE UNIQUE INDEX IF NOT EXISTS events_participation_id ON onsite.events(tenant_id, participation_event_id) WHERE participation_event_id IS NOT NULL;

-- "provisional" marca los tickets aceptados por supervisión antes de llegar la proyección.
ALTER TABLE onsite.ticket_projections ADD COLUMN IF NOT EXISTS provisional boolean NOT NULL DEFAULT false;

-- Rechazos de escaneo: sólo el motivo y dónde ocurrió; ni el QR ni datos personales.
CREATE TABLE IF NOT EXISTS onsite.scan_rejections(
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  event_id uuid REFERENCES onsite.events(id),
  scope_code text NOT NULL,
  reason text NOT NULL,
  actor_id uuid NOT NULL,
  device_id uuid,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS scan_rejections_event ON onsite.scan_rejections(event_id, occurred_at DESC);

-- Reglas de sesión: un scope de tipo 'session' es la sesión.
CREATE TABLE IF NOT EXISTS onsite.session_rules(
  scope_id uuid PRIMARY KEY REFERENCES onsite.scopes(id),
  access text NOT NULL CHECK (access IN ('open','restricted')),
  capacity int CHECK (capacity IS NULL OR capacity > 0),
  opens_at timestamptz,
  closes_at timestamptz,
  CHECK (opens_at IS NULL OR closes_at IS NULL OR opens_at < closes_at)
);
CREATE TABLE IF NOT EXISTS onsite.ticket_sessions(
  ticket_id text NOT NULL REFERENCES onsite.ticket_projections(ticket_id),
  scope_id uuid NOT NULL REFERENCES onsite.scopes(id),
  PRIMARY KEY (ticket_id, scope_id)
);
CREATE TABLE IF NOT EXISTS onsite.session_entries(
  id uuid PRIMARY KEY,
  ticket_id text NOT NULL,
  scope_id uuid NOT NULL REFERENCES onsite.scopes(id),
  actor_id uuid NOT NULL,
  device_id uuid,
  occurred_at timestamptz NOT NULL,
  UNIQUE (ticket_id, scope_id)
);

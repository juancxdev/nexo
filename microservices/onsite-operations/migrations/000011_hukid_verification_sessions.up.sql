CREATE TABLE IF NOT EXISTS onsite.hukid_verification_sessions(
  session_id text PRIMARY KEY,
  tenant_id uuid NOT NULL,
  event_id text NOT NULL,
  scope text NOT NULL,
  ticket_id text NOT NULL,
  status text NOT NULL CHECK(status IN ('pending','completed','expired')) DEFAULT 'pending',
  result text CHECK(result IN ('pass','fail') OR result IS NULL),
  idempotency_key text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  completed_at timestamptz,
  UNIQUE(tenant_id,idempotency_key)
);

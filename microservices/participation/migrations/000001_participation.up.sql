CREATE SCHEMA IF NOT EXISTS participation;
CREATE TABLE IF NOT EXISTS participation.forms (
  id uuid PRIMARY KEY, site_slug text NOT NULL, event_slug text NOT NULL, event_id text NOT NULL,
  event_title text NOT NULL, form_version_id uuid NOT NULL, consent_version text NOT NULL,
  binding_state text NOT NULL CHECK (binding_state IN ('scheduled','open','closed','cancelled')),
  consent_text text NOT NULL, schema jsonb NOT NULL DEFAULT '{}'::jsonb, capacity integer NOT NULL CHECK (capacity >= 0),
  confirmed_count integer NOT NULL DEFAULT 0, remaining integer NOT NULL CHECK (remaining >= 0), waitlist_enabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(site_slug,event_slug)
);
CREATE TABLE IF NOT EXISTS participation.registrations (
  id uuid PRIMARY KEY, form_id uuid NOT NULL REFERENCES participation.forms(id), state text NOT NULL,
  full_name text NOT NULL, email text NOT NULL, institution text NOT NULL DEFAULT '', answers jsonb NOT NULL DEFAULT '{}'::jsonb,
  self_access_token_ciphertext bytea NOT NULL, self_access_token_hash text NOT NULL UNIQUE, ticket_id text, ticket_jws text,
  ticket_state text, idempotency_key text NOT NULL, request_hash text NOT NULL, waitlist_position integer,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(form_id,idempotency_key), UNIQUE(form_id,email)
);
CREATE INDEX IF NOT EXISTS participation_registrations_ticket ON participation.registrations(ticket_id);

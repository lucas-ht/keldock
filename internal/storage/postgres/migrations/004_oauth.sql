CREATE TABLE access_mcp_clients (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  name text NOT NULL,
  redirect_uris jsonb NOT NULL,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  disabled_at timestamptz
);

CREATE TABLE access_oauth_requests (
  id text PRIMARY KEY,
  client_id text NOT NULL REFERENCES access_mcp_clients(id),
  redirect_uri text NOT NULL,
  state text NOT NULL,
  challenge text NOT NULL,
  resource text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz
);

CREATE TABLE access_mcp_grants (
  id text PRIMARY KEY,
  client_id text NOT NULL REFERENCES access_mcp_clients(id),
  user_id text NOT NULL REFERENCES access_users(id),
  user_version bigint NOT NULL,
  project_ids jsonb NOT NULL,
  resource text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  revoked_at timestamptz
);

CREATE TABLE access_oauth_codes (
  token_hash text PRIMARY KEY,
  grant_id text NOT NULL REFERENCES access_mcp_grants(id),
  redirect_uri text NOT NULL,
  challenge text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz
);

CREATE TABLE access_mcp_tokens (
  token_hash text PRIMARY KEY,
  grant_id text NOT NULL REFERENCES access_mcp_grants(id),
  kind text NOT NULL CHECK (kind IN ('access', 'refresh')),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX active_mcp_grant_tokens ON access_mcp_tokens(grant_id, kind);

CREATE TABLE github_apps (
  organization_id text PRIMARY KEY,
  app_id bigint NOT NULL CHECK (app_id > 0),
  client_id text NOT NULL,
  slug text NOT NULL,
  fingerprint text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  version bigint NOT NULL DEFAULT 1,
  permissions jsonb NOT NULL DEFAULT '{}',
  synced_at timestamptz,
  sync_error text NOT NULL DEFAULT ''
);

CREATE TABLE github_installations (
  organization_id text NOT NULL REFERENCES github_apps (organization_id),
  id bigint NOT NULL CHECK (id > 0),
  account_id bigint NOT NULL CHECK (account_id > 0),
  login text NOT NULL,
  active boolean NOT NULL,
  blocked boolean NOT NULL DEFAULT false,
  permissions jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id,id)
);

CREATE TABLE github_repositories (
  organization_id text NOT NULL,
  id bigint NOT NULL CHECK (id > 0),
  installation_id bigint NOT NULL,
  owner_id bigint NOT NULL CHECK (owner_id > 0),
  full_name text NOT NULL,
  project_id text NOT NULL DEFAULT '',
  active boolean NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id,id),
  FOREIGN KEY (organization_id,installation_id) REFERENCES github_installations (organization_id,id)
);

CREATE TABLE github_setup_requests (
  state_hash bytea PRIMARY KEY,
  organization_id text NOT NULL,
  user_id text NOT NULL REFERENCES access_users (id),
  user_version bigint NOT NULL,
  purpose text NOT NULL CHECK (purpose IN ('manifest','installation')),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  result_app_id bigint
);

CREATE TABLE github_events (
  organization_id text NOT NULL,
  delivery_id text NOT NULL,
  event text NOT NULL,
  payload_hash text NOT NULL,
  payload jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  attempt int NOT NULL DEFAULT 0,
  available_at timestamptz NOT NULL DEFAULT now(),
  lease_token text NOT NULL DEFAULT '',
  lease_until timestamptz,
  error text NOT NULL DEFAULT '',
  PRIMARY KEY (organization_id,delivery_id)
);
CREATE INDEX github_pending_events ON github_events (organization_id,available_at) WHERE processed_at IS NULL;

CREATE TABLE github_feedback (
  operation_id text PRIMARY KEY REFERENCES operations (id),
  organization_id text NOT NULL,
  sent_hash text NOT NULL DEFAULT '',
  check_id bigint NOT NULL DEFAULT 0,
  deployment_id bigint NOT NULL DEFAULT 0,
  available_at timestamptz NOT NULL DEFAULT now(),
  lease_token text NOT NULL DEFAULT '',
  lease_until timestamptz,
  attempt int NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE github_pr_comments (
  organization_id text NOT NULL,
  repository_id bigint NOT NULL,
  pull_request bigint NOT NULL,
  comment_id bigint NOT NULL,
  PRIMARY KEY (organization_id,repository_id,pull_request)
);

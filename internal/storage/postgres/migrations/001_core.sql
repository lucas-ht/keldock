CREATE TABLE projects (
  organization_id text NOT NULL,
  id text NOT NULL,
  name text NOT NULL,
  repository_id bigint NOT NULL CHECK (repository_id > 0),
  cluster_id text NOT NULL,
  PRIMARY KEY (organization_id, id),
  UNIQUE (organization_id, repository_id)
);

CREATE TABLE releases (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  project_id text NOT NULL,
  input_hash text NOT NULL,
  input jsonb NOT NULL,
  admitted_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id),
  UNIQUE (organization_id, project_id, input_hash)
);

CREATE TABLE pull_requests (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  number bigint NOT NULL CHECK (number > 0),
  head_commit text NOT NULL,
  head_repository_id bigint NOT NULL,
  closed boolean NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (organization_id, project_id, number),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id)
);

CREATE TABLE environments (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  project_id text NOT NULL,
  application text NOT NULL,
  cluster_id text NOT NULL,
  repository_id bigint NOT NULL,
  pull_request bigint NOT NULL DEFAULT 0,
  generation bigint NOT NULL DEFAULT 0 CHECK (generation >= 0),
  deleted boolean NOT NULL DEFAULT false,
  latest_run_id bigint NOT NULL DEFAULT 0,
  latest_run_attempt bigint NOT NULL DEFAULT 0,
  expires_at timestamptz,
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id),
  UNIQUE (organization_id, project_id, application, pull_request)
);

CREATE TABLE deployments (
  id text PRIMARY KEY,
  environment_id text NOT NULL REFERENCES environments (id),
  release_id text NOT NULL REFERENCES releases (id),
  generation bigint NOT NULL,
  config_hash text NOT NULL,
  selection jsonb NOT NULL,
  bundle text NOT NULL DEFAULT '',
  UNIQUE (environment_id, generation)
);

CREATE TABLE operations (
  id text PRIMARY KEY,
  environment_id text NOT NULL REFERENCES environments (id),
  deployment_id text REFERENCES deployments (id),
  generation bigint NOT NULL,
  kind text NOT NULL CHECK (kind IN ('deploy', 'delete')),
  state text NOT NULL CHECK (state IN ('pending', 'publishing', 'reconciling', 'ready', 'failed', 'canceled', 'superseded', 'deleted')),
  attempt bigint NOT NULL DEFAULT 0,
  lease_token text NOT NULL DEFAULT '',
  lease_until timestamptz,
  available_at timestamptz NOT NULL DEFAULT now(),
  error text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (environment_id, generation)
);
CREATE INDEX pending_operations ON operations (available_at, updated_at) WHERE state IN ('pending', 'publishing');

CREATE TABLE receipts (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  key text NOT NULL,
  request_hash text NOT NULL,
  response jsonb NOT NULL,
  PRIMARY KEY (organization_id, project_id, key)
);

CREATE TABLE observations (
  environment_id text PRIMARY KEY REFERENCES environments (id),
  generation bigint NOT NULL,
  report jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id text NOT NULL,
  project_id text NOT NULL,
  actor text NOT NULL,
  event text NOT NULL,
  target text NOT NULL,
  generation bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  project_id text NOT NULL,
  payload_hash text NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);

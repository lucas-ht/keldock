CREATE TABLE access_clusters (
  organization_id text NOT NULL,
  id text NOT NULL,
  name text NOT NULL,
  revision bigint NOT NULL DEFAULT 1,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE access_enrollments (
  token_hash text PRIMARY KEY,
  organization_id text NOT NULL,
  cluster_id text NOT NULL,
  revision bigint NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, cluster_id) REFERENCES access_clusters(organization_id, id)
);

CREATE TABLE access_agents (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  cluster_id text NOT NULL,
  revision bigint NOT NULL,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  session_id text NOT NULL DEFAULT '',
  session_until timestamptz,
  last_seen_at timestamptz,
  version text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  rotated_at timestamptz,
  revoked_at timestamptz,
  FOREIGN KEY (organization_id, cluster_id) REFERENCES access_clusters(organization_id, id)
);
CREATE UNIQUE INDEX one_active_cluster_agent ON access_agents(organization_id, cluster_id) WHERE revoked_at IS NULL;

CREATE TABLE database_profiles (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  configuration jsonb NOT NULL,
  PRIMARY KEY (organization_id, project_id),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects(organization_id,id)
);

CREATE TABLE database_allocations (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  id text NOT NULL,
  environment_id text NOT NULL REFERENCES environments(id),
  alias text NOT NULL,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation>0),
  intent jsonb NOT NULL,
  observation jsonb,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id,project_id,id),
  UNIQUE(environment_id,alias)
);

CREATE TABLE database_receipts (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  key text NOT NULL,
  request_hash text NOT NULL,
  response jsonb NOT NULL,
  PRIMARY KEY(organization_id,project_id,key)
);

CREATE TABLE access_database_clusters (LIKE access_cloud_clusters INCLUDING ALL);
CREATE TABLE access_database_enrollments (LIKE access_cloud_enrollments INCLUDING ALL);
ALTER TABLE access_database_enrollments ADD FOREIGN KEY (organization_id,cluster_id)
  REFERENCES access_database_clusters(organization_id,id);
CREATE TABLE access_database_agents (LIKE access_cloud_agents INCLUDING ALL);
ALTER TABLE access_database_agents ADD FOREIGN KEY (organization_id,cluster_id)
  REFERENCES access_database_clusters(organization_id,id);

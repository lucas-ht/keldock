CREATE TABLE cloud_intents (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  cluster_id text NOT NULL,
  id text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('connection','resource','binding','domain','backup')),
  generation bigint NOT NULL CHECK (generation > 0),
  intent jsonb NOT NULL,
  external_key text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id,project_id,id),
  FOREIGN KEY (organization_id,project_id) REFERENCES projects(organization_id,id)
);
CREATE UNIQUE INDEX one_cloud_hostname ON cloud_intents(external_key)
  WHERE kind='domain' AND external_key<>'';
CREATE INDEX cloud_cluster_inventory ON cloud_intents(organization_id,cluster_id,id);

CREATE TABLE cloud_receipts (
  organization_id text NOT NULL,
  project_id text NOT NULL,
  key text NOT NULL,
  request_hash text NOT NULL,
  response jsonb NOT NULL,
  PRIMARY KEY (organization_id,project_id,key)
);

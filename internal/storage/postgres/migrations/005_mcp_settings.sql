CREATE TABLE access_mcp_settings (
  organization_id text PRIMARY KEY,
  enabled boolean NOT NULL DEFAULT false,
  version bigint NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL DEFAULT now()
);

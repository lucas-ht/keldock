ALTER TABLE projects ADD COLUMN preview_policy jsonb NOT NULL
  DEFAULT '{"profile":"stateless-v1","ttlSeconds":86400,"maxActive":10}';
ALTER TABLE environments ADD COLUMN kind text NOT NULL DEFAULT 'staging'
  CHECK (kind IN ('staging','preview','production'));
UPDATE environments SET kind='preview' WHERE pull_request>0;
ALTER TABLE environments ADD COLUMN deletion_reason text NOT NULL DEFAULT '';
UPDATE environments SET deletion_reason='pull_request_closed' WHERE deleted AND pull_request>0;
CREATE UNIQUE INDEX one_project_pull_preview ON environments(organization_id,project_id,pull_request) WHERE pull_request>0;

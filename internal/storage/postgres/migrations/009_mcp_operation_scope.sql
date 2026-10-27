ALTER TABLE access_oauth_requests ADD COLUMN scope text NOT NULL DEFAULT 'keldock:read'
  CHECK (scope IN ('keldock:read','keldock:read keldock:operate'));
ALTER TABLE access_mcp_grants ADD COLUMN scope text NOT NULL DEFAULT 'keldock:read'
  CHECK (scope IN ('keldock:read','keldock:read keldock:operate'));

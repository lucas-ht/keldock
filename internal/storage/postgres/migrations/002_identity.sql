CREATE TABLE access_users (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  email text NOT NULL CHECK (email = lower(email)),
  name text NOT NULL,
  password_hash text NOT NULL DEFAULT '',
  role text NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
  status text NOT NULL CHECK (status IN ('invited', 'active', 'disabled')),
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz,
  UNIQUE (organization_id, email)
);

CREATE TABLE access_user_projects (
  user_id text NOT NULL REFERENCES access_users(id),
  organization_id text NOT NULL,
  project_id text NOT NULL,
  PRIMARY KEY (user_id, project_id),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects(organization_id, id)
);

CREATE TABLE access_invitations (
  token_hash text PRIMARY KEY,
  user_id text NOT NULL REFERENCES access_users(id),
  kind text NOT NULL CHECK (kind IN ('activate', 'reset')),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE access_sessions (
  token_hash text PRIMARY KEY,
  user_id text NOT NULL REFERENCES access_users(id),
  user_version bigint NOT NULL,
  expires_at timestamptz NOT NULL,
  last_used_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz
);
CREATE INDEX active_user_sessions ON access_sessions(user_id) WHERE revoked_at IS NULL;

CREATE TABLE access_login_limits (
  organization_id text NOT NULL,
  bucket text NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  window_start timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, bucket)
);

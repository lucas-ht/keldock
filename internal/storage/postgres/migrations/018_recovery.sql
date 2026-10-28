-- Restore must set this flag before a restored database can serve or dispatch.
CREATE TABLE recovery_state (
  organization_id text PRIMARY KEY,
  fenced boolean NOT NULL DEFAULT true,
  snapshot_id text NOT NULL,
  restored_at timestamptz NOT NULL DEFAULT now(),
  resumed_at timestamptz
);

-- A component's shared IAM role is a union across bindings. Fence its projection
-- by the project intent sequence, not by unrelated per-binding generations.
ALTER TABLE projects ADD COLUMN cloud_revision bigint NOT NULL DEFAULT 0 CHECK (cloud_revision>=0);

ALTER TABLE github_pr_comments ALTER COLUMN comment_id SET DEFAULT 0;
ALTER TABLE github_pr_comments ADD COLUMN lease_token text NOT NULL DEFAULT '';
ALTER TABLE github_pr_comments ADD COLUMN lease_until timestamptz;
CREATE INDEX github_feedback_available ON github_feedback (organization_id,available_at);

CREATE TABLE operation_budgets (
  organization_id text NOT NULL,
  actor text NOT NULL,
  window_start timestamptz NOT NULL,
  count integer NOT NULL CHECK (count BETWEEN 1 AND 30),
  PRIMARY KEY (organization_id,actor,window_start)
);

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func environmentKind(pull int64) string {
	if pull > 0 {
		return "preview"
	}
	return "staging"
}

// previewTarget runs while holding the project lock: one PR keeps its ID even if its app is renamed.
func previewTarget(ctx context.Context, tx pgx.Tx, scope v1.Scope, application string, pull int64, policy v1.PreviewPolicy) (string, error) {
	if pull == 0 {
		return v1.EnvironmentID(scope, application, pull), nil
	}
	var id, state string
	err := tx.QueryRow(ctx, `SELECT e.id,o.state FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation WHERE e.organization_id=$1 AND e.project_id=$2 AND e.pull_request=$3`, scope.OrganizationID, scope.ProjectID, pull).Scan(&id, &state)
	if err == nil && state != "deleted" {
		return id, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var reserved int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation WHERE e.organization_id=$1 AND e.project_id=$2 AND e.pull_request>0 AND o.state<>'deleted'`, scope.OrganizationID, scope.ProjectID).Scan(&reserved)
	if err != nil {
		return "", err
	}
	if reserved >= policy.MaxActive {
		return "", ErrQuota
	}
	if id != "" {
		return id, nil
	}
	return v1.EnvironmentID(scope, application, pull), nil
}

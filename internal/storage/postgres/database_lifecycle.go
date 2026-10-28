package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// ReconfigureDatabases records a trusted binding change while preserving admitted images.
func ReconfigureDatabases(ctx context.Context, tx pgx.Tx, scope v1.Scope, release string, selection v1.Placement) error {
	selection.Generation++
	selection.DeploymentID = newID("dep")
	if _, err := selectDeployment(ctx, tx, scope, release, selection); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE environments SET generation=$2 WHERE id=$1 AND organization_id=$3 AND project_id=$4`, selection.EnvironmentID, selection.Generation, scope.OrganizationID, scope.ProjectID)
	return err
}

//go:build integration

package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dbmodel "github.com/lucas-ht/keldock/internal/database"
)

func seedMCPResource(t *testing.T, ctx context.Context, f *apiFixture, environment string) {
	t.Helper()
	allocation := dbmodel.Allocation{Scope: f.scope, ID: dbmodel.ID(environment, "data"), EnvironmentID: environment, Alias: "data", ClusterID: "keldock-m1", Namespace: "private-database-namespace", Profile: "development", Generation: 1, SourceUID: "private-source-identity", Action: "retain", Backup: &dbmodel.BackupIntent{DestinationID: "private-archive-reference"}}
	evidence := dbmodel.Evidence{Scope: f.scope, ID: allocation.ID, Generation: 1, State: "retained", UID: "private-source-identity", ObservedAt: time.Now().Add(-2 * time.Minute), Backup: &dbmodel.BackupEvidence{State: "ready", Reason: "BaseBackupAndWALObserved", WALHealthy: true, WALObjects: true}}
	intent, err := json.Marshal(allocation)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Pool().Exec(ctx, `INSERT INTO database_allocations(organization_id,project_id,id,environment_id,alias,intent,observation) VALUES ($1,$2,$3,$4,'data',$5,$6)`, f.scope.OrganizationID, f.scope.ProjectID, allocation.ID, environment, intent, observation); err != nil {
		t.Fatal(err)
	}
}

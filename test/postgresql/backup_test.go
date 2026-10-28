//go:build integration

package postgresql_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
	databasestore "github.com/lucas-ht/keldock/internal/database/store"
)

func backupAndRestore(t *testing.T, f *fixture, staging *v1.ProjectedDeployment) {
	t.Helper()
	items, err := f.databaseStore.List(f.ctx, f.principal, f.scope.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	var allocation database.Allocation
	for _, item := range items {
		if item.EnvironmentID == staging.Selection.EnvironmentID {
			allocation = item
		}
	}
	if allocation.SourceUID == "" {
		t.Fatal("source identity was not durably observed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	setup := exec.CommandContext(f.ctx, "node", "tooling/alpha/backup-store.mjs", "--allocation", allocation.ID, "--organization", f.scope.OrganizationID, "--project", f.scope.ProjectID, "--uid", allocation.SourceUID)
	setup.Dir = root
	if output, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("supplied backup fixture: %s (%v)", output, err)
	} else {
		t.Log(string(output))
	}
	data, err := os.ReadFile(filepath.Join(root, ".local/m6/backup-store/destinations", allocation.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var destination database.Destination
	if err := json.Unmarshal(data, &destination); err != nil {
		t.Fatal(err)
	}
	f.policy.Backups = map[string]database.Destination{"supplied": destination}
	if err := f.store.SeedDatabasePolicy(f.ctx, f.policy); err != nil {
		t.Fatal(err)
	}
	f.databaseStore.Policies[f.scope.ProjectID] = f.policy
	f.provisioner.Policies[f.scope.ProjectID] = f.policy
	allocation = mutateDatabase(t, f, databasestore.Command{ID: allocation.ID, Generation: allocation.Generation, Action: "backup", DestinationID: "supplied"})
	allocation = waitBackup(t, f, allocation.ID, false, time.Time{})
	t.Log("Completed real base backup and observed archive WAL with scoped supplied identities")
	committed := time.Now().UTC()
	for deadline := time.Now().Add(30 * time.Second); ; {
		response := request(t, staging.Selection.Hostname, "POST", "/markers/after-base-committed")
		status := response.StatusCode
		closeResponse(t, response)
		if status == http.StatusCreated {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("post-base committed marker write failed")
		}
		time.Sleep(time.Second)
	}
	if err := kube(t, f, "exec", "-n", allocation.Namespace, "database-1", "-c", "postgres", "--", "psql", "-U", "postgres", "-d", "app", "-c", "SELECT pg_switch_wal()"); err != nil {
		t.Fatal(err)
	}
	allocation = waitBackup(t, f, allocation.ID, false, committed)
	allocation = mutateDatabase(t, f, databasestore.Command{ID: allocation.ID, Generation: allocation.Generation, Action: "restore"})
	allocation = waitBackup(t, f, allocation.ID, true, committed)
	e := allocation.Observation.Backup
	query := exec.CommandContext(f.ctx, "kubectl", "--kubeconfig", f.kubeconfig, "--context", "k3d-keldock-m6", "-n", e.RestoreNamespace, "exec", e.RestoreName+"-1", "-c", "postgres", "--", "psql", "-U", "postgres", "-d", "app", "-tAc", "SELECT count(*) FROM events WHERE kind='marker' AND payload='after-base-committed'")
	output, err := query.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "1" {
		t.Fatalf("isolated restore did not replay committed post-base WAL: %s (%v)", output, err)
	}
	t.Logf("Isolated restore %s/%s (%s) replayed a marker committed after its base backup", e.RestoreNamespace, e.RestoreName, e.RestoreUID)
}

func mutateDatabase(t *testing.T, f *fixture, command databasestore.Command) database.Allocation {
	t.Helper()
	data, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequestWithContext(f.ctx, "POST", f.publicURL+"/v1/projects/"+f.scope.ProjectID+"/databases", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", f.publicURL)
	r.Header.Set("X-Keldock-CSRF", access.CSRF(f.session))
	r.Header.Set("Idempotency-Key", fmt.Sprintf("database-backup-%s-%d", command.Action, command.Generation))
	r.AddCookie(&http.Cookie{Name: "keldock_session", Value: f.session})
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("database lifecycle HTTP status %d", response.StatusCode)
	}
	var allocation database.Allocation
	if err := json.NewDecoder(response.Body).Decode(&allocation); err != nil {
		t.Fatal(err)
	}
	return allocation
}

func waitBackup(t *testing.T, f *fixture, id string, restore bool, after time.Time) database.Allocation {
	t.Helper()
	var last database.Allocation
	for deadline := time.Now().Add(12 * time.Minute); time.Now().Before(deadline); {
		if err := f.provisioner.Step(f.ctx); err != nil {
			t.Fatal(err)
		}
		items, err := f.databaseStore.List(f.ctx, f.principal, f.scope.ProjectID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == id {
				last = item
			}
		}
		if last.Observation != nil && last.Observation.Backup != nil {
			e := last.Observation.Backup
			if e.State == "failed" {
				t.Fatalf("backup failed: %s", e.Reason)
			}
			if e.State == "ready" && (!restore || e.RestoreReady) && (after.IsZero() || (e.LastArchivedAt != nil && !e.LastArchivedAt.Before(after))) {
				return last
			}
		}
		time.Sleep(2 * time.Second)
	}
	data, err := json.Marshal(last.Observation)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("backup did not converge: %s", data)
	return last
}

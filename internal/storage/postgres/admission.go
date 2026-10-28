package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
)

// Admit atomically records immutable selection, receipt, audit and durable dispatch.
// Authentication, provenance and target policy must already have been checked by admission.
func (store *Store) Admit(ctx context.Context, scope v1.Scope, actor, key, hostname string, input v1.SubmitRelease) (v1.Receipt, error) {
	return store.admit(ctx, scope, actor, key, hostname, "", input)
}

// AdmitToDomain derives routing from the stable stored target and operator-owned domain.
func (store *Store) AdmitToDomain(ctx context.Context, scope v1.Scope, actor, key, domain string, input v1.SubmitRelease) (v1.Receipt, error) {
	return store.admit(ctx, scope, actor, key, "", domain, input)
}

func (store *Store) admit(ctx context.Context, scope v1.Scope, actor, key, hostname, domain string, input v1.SubmitRelease) (v1.Receipt, error) {
	var receipt v1.Receipt
	if err := input.Validate(); err != nil {
		return receipt, err
	}
	if scope.ProjectID != input.ProjectID || !v1.ValidName(scope.OrganizationID) || len(key) < 8 || len(key) > 128 || actor == "" {
		return receipt, ErrDenied
	}
	requestHash, err := provenance.Hash(input)
	if err != nil {
		return receipt, err
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer rollback(ctx, tx)
	var cluster string
	var repository int64
	var policyData []byte
	var policy v1.PreviewPolicy
	if err := tx.QueryRow(ctx, `SELECT cluster_id,repository_id,preview_policy FROM projects WHERE organization_id=$1 AND id=$2 FOR UPDATE`, scope.OrganizationID, scope.ProjectID).Scan(&cluster, &repository, &policyData); err != nil {
		return receipt, ErrDenied
	}
	if err := json.Unmarshal(policyData, &policy); err != nil {
		return receipt, err
	}
	if policy, err = policy.Normalize(); err != nil {
		return receipt, err
	}
	if repository != input.Source.RepositoryID {
		return receipt, ErrDenied
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, scope.OrganizationID+"/"+scope.ProjectID+"/"+key); err != nil {
		return receipt, err
	}
	var savedHash string
	var saved []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response FROM receipts WHERE organization_id=$1 AND project_id=$2 AND key=$3`, scope.OrganizationID, scope.ProjectID, key).Scan(&savedHash, &saved)
	if err == nil {
		if savedHash != requestHash {
			return receipt, ErrConflict
		}
		if err := json.Unmarshal(saved, &receipt); err != nil {
			return receipt, err
		}
		return receipt, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	if input.Source.PullRequest > 0 {
		if policy.Profile == "disabled" {
			return receipt, ErrDenied
		}
		var head string
		var headRepository int64
		var closed bool
		err := tx.QueryRow(ctx, `SELECT head_commit,head_repository_id,closed FROM pull_requests WHERE organization_id=$1 AND project_id=$2 AND number=$3 FOR SHARE`, scope.OrganizationID, scope.ProjectID, input.Source.PullRequest).Scan(&head, &headRepository, &closed)
		if err != nil || closed || headRepository != repository || head != input.Source.HeadCommit {
			return receipt, ErrStale
		}
	}
	environmentID, err := previewTarget(ctx, tx, scope, input.Definition.Name, input.Source.PullRequest, policy)
	if err != nil {
		return receipt, err
	}
	if domain != "" {
		prefix := "preview-"
		if input.Source.PullRequest == 0 {
			prefix = "staging-"
		}
		hostname = prefix + strings.TrimPrefix(environmentID, "env-")[:12] + "." + domain
	}
	environmentHash := strings.TrimPrefix(environmentID, "env-")
	_, err = tx.Exec(ctx, `INSERT INTO environments (id,organization_id,project_id,application,cluster_id,repository_id,pull_request,kind) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, environmentID, scope.OrganizationID, scope.ProjectID, input.Definition.Name, cluster, repository, input.Source.PullRequest, environmentKind(input.Source.PullRequest))
	if err != nil {
		return receipt, err
	}
	var generation, run, attempt int64
	var deleted bool
	var expiresAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT generation,deleted,latest_run_id,latest_run_attempt,expires_at FROM environments WHERE id=$1 FOR UPDATE`, environmentID).Scan(&generation, &deleted, &run, &attempt, &expiresAt); err != nil {
		return receipt, err
	}
	if deleted || (expiresAt != nil && !expiresAt.After(time.Now())) || input.Source.RunID < run || (input.Source.RunID == run && input.Source.RunAttempt <= attempt) {
		return receipt, ErrStale
	}
	generation++
	releaseID, deploymentID, operationID := newID("rel"), newID("dep"), newID("op")
	var admittedRelease string
	data, err := json.Marshal(input)
	if err != nil {
		return receipt, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO releases (id,organization_id,project_id,input_hash,input) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (organization_id,project_id,input_hash) DO UPDATE SET input_hash=EXCLUDED.input_hash RETURNING id`, releaseID, scope.OrganizationID, scope.ProjectID, requestHash, data).Scan(&admittedRelease)
	if err != nil {
		return receipt, err
	}
	selection := compiler.Selection{EnvironmentID: environmentID, DeploymentID: deploymentID, Generation: generation, Namespace: "ke-" + environmentHash, Hostname: hostname}
	if err := resolveDatabases(ctx, tx, scope, admittedRelease, &selection); err != nil {
		return receipt, err
	}
	if err := inheritCapabilities(ctx, tx, scope, environmentID, &selection); err != nil {
		return receipt, err
	}
	config, err := json.Marshal(selection)
	if err != nil {
		return receipt, err
	}
	configHash, err := provenance.Hash(selection)
	if err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO deployments (id,environment_id,release_id,generation,config_hash,selection) VALUES ($1,$2,$3,$4,$5,$6)`, deploymentID, environmentID, admittedRelease, generation, configHash, config); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `UPDATE operations SET state='superseded',lease_token='',lease_until=NULL,updated_at=now() WHERE environment_id=$1 AND state NOT IN ('superseded','deleted','canceled')`, environmentID); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO operations (id,environment_id,deployment_id,generation,kind,state) VALUES ($1,$2,$3,$4,'deploy','pending')`, operationID, environmentID, deploymentID, generation); err != nil {
		return receipt, err
	}
	var expires *time.Time
	if input.Source.PullRequest > 0 {
		value := time.Now().UTC().Add(time.Duration(policy.TTLSeconds) * time.Second)
		expires = &value
	}
	if _, err := tx.Exec(ctx, `UPDATE environments SET generation=$2,latest_run_id=$3,latest_run_attempt=$4,expires_at=$5,application=$6 WHERE id=$1`, environmentID, generation, input.Source.RunID, input.Source.RunAttempt, expires, input.Definition.Name); err != nil {
		return receipt, err
	}
	receipt = v1.Receipt{APIVersion: v1.APIVersion, ReleaseID: admittedRelease, EnvironmentID: environmentID, DeploymentID: deploymentID, OperationID: operationID, Generation: generation, StatusURL: "/v1/projects/" + scope.ProjectID + "/operations/" + operationID}
	response, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO receipts VALUES ($1,$2,$3,$4,$5)`, scope.OrganizationID, scope.ProjectID, key, requestHash, response); err != nil {
		return receipt, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,project_id,actor,event,target,generation) VALUES ($1,$2,$3,'release.admitted',$4,$5)`, scope.OrganizationID, scope.ProjectID, actor, environmentID, generation); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}

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

// ReleaseInput reads immutable admitted inputs inside the requested ownership scope.
func (store *Store) ReleaseInput(ctx context.Context, scope v1.Scope, id string) (v1.SubmitRelease, error) {
	var input v1.SubmitRelease
	var data []byte
	err := store.pool.QueryRow(ctx, `SELECT input FROM releases WHERE organization_id=$1 AND project_id=$2 AND id=$3`, scope.OrganizationID, scope.ProjectID, id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return input, ErrNotFound
	}
	if err != nil {
		return input, err
	}
	err = json.Unmarshal(data, &input)
	return input, err
}

// SelectRelease records a new immutable deployment without accepting any new CI inputs.
func (store *Store) SelectRelease(ctx context.Context, scope v1.Scope, actor string, request v1.ReleaseSelection, authorize MutationAuthority) (v1.Receipt, error) {
	var receipt v1.Receipt
	if authorize == nil || request.Generation < 1 || request.ReleaseID == "" || request.EnvironmentID == "" {
		return receipt, ErrDenied
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer rollback(ctx, tx)
	if err = authorize(ctx, tx); err != nil {
		return receipt, err
	}
	intent := struct {
		Action  string
		Request v1.ReleaseSelection
	}{"select", request}
	receipt, replay, err := mutationReceipt(ctx, tx, scope, actor, request.RequestKey, intent)
	if err != nil {
		return receipt, err
	}
	if replay {
		return receipt, tx.Commit(ctx)
	}
	policy, err := mutationPolicy(ctx, tx, scope)
	if err != nil {
		return receipt, err
	}
	var data []byte
	if err = tx.QueryRow(ctx, `SELECT input FROM releases WHERE organization_id=$1 AND project_id=$2 AND id=$3`, scope.OrganizationID, scope.ProjectID, request.ReleaseID).Scan(&data); errors.Is(err, pgx.ErrNoRows) {
		return receipt, ErrNotFound
	} else if err != nil {
		return receipt, err
	}
	var input v1.SubmitRelease
	if err = json.Unmarshal(data, &input); err != nil {
		return receipt, err
	}
	var current, pull, repository int64
	var deleted, fresh bool
	var kind, application, reason, state string
	err = tx.QueryRow(ctx, `SELECT e.generation,e.pull_request,e.repository_id,e.deleted,e.kind,e.application,e.deletion_reason,o.state,coalesce(e.expires_at>clock_timestamp(),true) FROM environments e JOIN operations o ON o.environment_id=e.id AND o.generation=e.generation WHERE e.id=$1 AND e.organization_id=$2 AND e.project_id=$3 FOR UPDATE OF e,o`, request.EnvironmentID, scope.OrganizationID, scope.ProjectID).Scan(&current, &pull, &repository, &deleted, &kind, &application, &reason, &state, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, ErrNotFound
	}
	if err != nil {
		return receipt, err
	}
	if kind != "preview" && kind != "staging" {
		return receipt, ErrDenied
	}
	if current != request.Generation || (deleted && (kind != "preview" || reason != "expired" || state != "deleted")) || (!deleted && !fresh) {
		return receipt, ErrStale
	}
	if repository != input.Source.RepositoryID || pull != input.Source.PullRequest || (pull == 0 && application != input.Definition.Name) {
		return receipt, ErrDenied
	}
	if pull > 0 {
		if policy.Profile == "disabled" {
			return receipt, ErrDenied
		}
		var allowed bool
		if err = tx.QueryRow(ctx, `SELECT NOT closed AND head_commit=$4 AND head_repository_id=$5 FROM pull_requests WHERE organization_id=$1 AND project_id=$2 AND number=$3 FOR SHARE`, scope.OrganizationID, scope.ProjectID, pull, input.Source.HeadCommit, repository).Scan(&allowed); err != nil || !allowed {
			return receipt, ErrStale
		}
		if deleted {
			if _, err = previewTarget(ctx, tx, scope, input.Definition.Name, pull, policy); err != nil {
				return receipt, err
			}
		}
	}
	var hostname string
	if err = tx.QueryRow(ctx, `SELECT selection->>'hostname' FROM deployments WHERE environment_id=$1 ORDER BY generation DESC LIMIT 1`, request.EnvironmentID).Scan(&hostname); err != nil {
		return receipt, err
	}
	selection := compiler.Selection{EnvironmentID: request.EnvironmentID, DeploymentID: newID("dep"), Generation: current + 1, Namespace: "ke-" + strings.TrimPrefix(request.EnvironmentID, "env-"), Hostname: hostname}
	receipt, err = selectDeployment(ctx, tx, scope, request.ReleaseID, selection)
	if err != nil {
		return receipt, err
	}
	var expires *time.Time
	if pull > 0 {
		value := time.Now().UTC().Add(time.Duration(policy.TTLSeconds) * time.Second)
		expires = &value
	}
	if _, err = tx.Exec(ctx, `UPDATE environments SET generation=$2,deleted=false,deletion_reason='',expires_at=$3,application=$4 WHERE id=$1`, request.EnvironmentID, selection.Generation, expires, input.Definition.Name); err != nil {
		return receipt, err
	}
	if err = saveMutation(ctx, tx, scope, actor, request.RequestKey, intent, receipt, "release.selected"); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}

func selectDeployment(ctx context.Context, tx pgx.Tx, scope v1.Scope, release string, selection compiler.Selection) (v1.Receipt, error) {
	var receipt v1.Receipt
	if err := inheritCapabilities(ctx, tx, scope, selection.EnvironmentID, &selection); err != nil {
		return receipt, err
	}
	data, err := json.Marshal(selection)
	if err != nil {
		return receipt, err
	}
	hash, err := provenance.Hash(selection)
	if err != nil {
		return receipt, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployments(id,environment_id,release_id,generation,config_hash,selection) VALUES($1,$2,$3,$4,$5,$6)`, selection.DeploymentID, selection.EnvironmentID, release, selection.Generation, hash, data); err != nil {
		return receipt, err
	}
	if _, err = tx.Exec(ctx, `UPDATE operations SET state='superseded',lease_token='',lease_until=NULL,updated_at=now() WHERE environment_id=$1 AND state NOT IN ('deleted','superseded')`, selection.EnvironmentID); err != nil {
		return receipt, err
	}
	receipt = v1.Receipt{APIVersion: v1.APIVersion, ReleaseID: release, EnvironmentID: selection.EnvironmentID, DeploymentID: selection.DeploymentID, OperationID: newID("op"), Generation: selection.Generation}
	receipt.StatusURL = "/v1/projects/" + scope.ProjectID + "/operations/" + receipt.OperationID
	_, err = tx.Exec(ctx, `INSERT INTO operations(id,environment_id,deployment_id,generation,kind,state) VALUES($1,$2,$3,$4,'deploy','pending')`, receipt.OperationID, receipt.EnvironmentID, receipt.DeploymentID, receipt.Generation)
	return receipt, err
}

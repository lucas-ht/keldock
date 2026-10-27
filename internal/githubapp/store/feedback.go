package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Feedback is a leased projection of durable deployment state, independent of CI.
type Feedback struct {
	Operation                           v1.Operation
	Release                             v1.SubmitRelease
	Report                              *v1.Observation
	CurrentGeneration                   int64
	CurrentKind, CurrentState, PullHead string
	Closed, LatestDeployment            bool
	Bundle, Hostname                    string
	CheckID, DeploymentID               int64
	SentHash, Lease                     string
	Attempt                             int
}

// DiscoverFeedback repairs a missed enqueue from the authoritative operation journal.
func (repository *Repository) DiscoverFeedback(ctx context.Context) error {
	_, err := repository.pool.Exec(ctx, `INSERT INTO github_feedback (operation_id,organization_id)
SELECT o.id,e.organization_id FROM operations o JOIN environments e ON e.id=o.environment_id
WHERE e.organization_id=$1 AND o.kind='deploy' ON CONFLICT DO NOTHING`, repository.Organization)
	return err
}

// ClaimFeedback snapshots one operation under an expiring delivery lease.
func (repository *Repository) ClaimFeedback(ctx context.Context) (*Feedback, error) {
	item := Feedback{Lease: access.NewID("feedback-lease")}
	item.Operation.OrganizationID = repository.Organization
	err := repository.pool.QueryRow(ctx, `UPDATE github_feedback SET lease_token=$2,lease_until=now()+interval '3 minutes',attempt=attempt+1
WHERE operation_id=(SELECT operation_id FROM github_feedback WHERE organization_id=$1 AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY available_at,operation_id FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING operation_id,check_id,deployment_id,sent_hash,attempt`, repository.Organization, item.Lease).Scan(&item.Operation.ID, &item.CheckID, &item.DeploymentID, &item.SentHash, &item.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var input, report []byte
	err = repository.pool.QueryRow(ctx, `SELECT e.project_id,e.id,o.deployment_id,o.generation,o.kind,o.state,o.updated_at,r.input,d.bundle,coalesce(d.selection->>'hostname',''),e.generation,current.kind,current.state,
coalesce(p.head_commit,''),coalesce(p.closed,false),d.generation=(SELECT max(latest.generation) FROM deployments latest WHERE latest.environment_id=e.id),
CASE WHEN v.generation=o.generation THEN v.report ELSE NULL END
FROM operations o JOIN environments e ON e.id=o.environment_id JOIN deployments d ON d.id=o.deployment_id JOIN releases r ON r.id=d.release_id
JOIN operations current ON current.environment_id=e.id AND current.generation=e.generation
LEFT JOIN observations v ON v.environment_id=e.id LEFT JOIN pull_requests p ON p.organization_id=e.organization_id AND p.project_id=e.project_id AND p.number=e.pull_request
WHERE o.id=$1 AND e.organization_id=$2`, item.Operation.ID, repository.Organization).Scan(&item.Operation.ProjectID, &item.Operation.EnvironmentID, &item.Operation.DeploymentID, &item.Operation.Generation, &item.Operation.Kind, &item.Operation.State, &item.Operation.UpdatedAt, &input, &item.Bundle, &item.Hostname, &item.CurrentGeneration, &item.CurrentKind, &item.CurrentState, &item.PullHead, &item.Closed, &item.LatestDeployment, &report)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(input, &item.Release); err != nil {
		return nil, err
	}
	if len(report) > 0 {
		if err := json.Unmarshal(report, &item.Report); err != nil {
			return nil, err
		}
	}
	return &item, nil
}

// SaveFeedbackIDs records each remote effect before attempting the next one.
func (repository *Repository) SaveFeedbackIDs(ctx context.Context, item Feedback) error {
	result, err := repository.pool.Exec(ctx, `UPDATE github_feedback SET check_id=$4,deployment_id=$5 WHERE organization_id=$1 AND operation_id=$2 AND lease_token=$3 AND lease_until>now()`, repository.Organization, item.Operation.ID, item.Lease, item.CheckID, item.DeploymentID)
	if err == nil && result.RowsAffected() != 1 {
		return access.ErrConflict
	}
	return err
}

// FinishFeedback records delivery health without exposing response bodies or credentials.
func (repository *Repository) FinishFeedback(ctx context.Context, item Feedback, hash string, retry time.Duration, success bool) error {
	message := ""
	if !success {
		message = "GitHub feedback unavailable; delivery will retry"
	}
	result, err := repository.pool.Exec(ctx, `UPDATE github_feedback SET sent_hash=CASE WHEN $5 THEN $4 ELSE sent_hash END,error=$6,available_at=now()+$7::interval,lease_token='',lease_until=NULL,updated_at=now(),attempt=CASE WHEN $5 THEN 0 ELSE attempt END WHERE organization_id=$1 AND operation_id=$2 AND lease_token=$3`, repository.Organization, item.Operation.ID, item.Lease, hash, success, message, retry.String())
	if err == nil && result.RowsAffected() != 1 {
		return access.ErrConflict
	}
	return err
}

// ClaimComment serializes the single PR comment across competing deployment operations.
func (repository *Repository) ClaimComment(ctx context.Context, repo, number int64, lease string) (int64, error) {
	var id int64
	err := repository.pool.QueryRow(ctx, `INSERT INTO github_pr_comments (organization_id,repository_id,pull_request,lease_token,lease_until) VALUES ($1,$2,$3,$4,now()+interval '3 minutes')
ON CONFLICT (organization_id,repository_id,pull_request) DO UPDATE SET lease_token=EXCLUDED.lease_token,lease_until=EXCLUDED.lease_until WHERE github_pr_comments.lease_until IS NULL OR github_pr_comments.lease_until<now()
RETURNING comment_id`, repository.Organization, repo, number, lease).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, access.ErrLimited
	}
	return id, err
}

// FinishComment persists a recovered/created ID and releases only the current lease.
func (repository *Repository) FinishComment(ctx context.Context, repo, number, id int64, lease string) error {
	result, err := repository.pool.Exec(ctx, `UPDATE github_pr_comments SET comment_id=CASE WHEN $4::bigint>0 THEN $4::bigint ELSE comment_id END,lease_token='',lease_until=NULL WHERE organization_id=$1 AND repository_id=$2 AND pull_request=$3 AND lease_token=$5`, repository.Organization, repo, number, id, lease)
	if err == nil && result.RowsAffected() != 1 {
		return access.ErrConflict
	}
	return err
}

// CommentCurrent rechecks latest selection after acquiring the cross-operation lease.
func (repository *Repository) CommentCurrent(ctx context.Context, item Feedback) (bool, error) {
	var current bool
	err := repository.pool.QueryRow(ctx, `SELECT d.generation=(SELECT max(latest.generation) FROM deployments latest WHERE latest.environment_id=e.id) AND (e.generation=o.generation OR e.deleted) AND
(e.pull_request=0 OR p.head_commit=$3 OR e.deleted) FROM operations o JOIN environments e ON e.id=o.environment_id JOIN deployments d ON d.id=o.deployment_id
LEFT JOIN pull_requests p ON p.organization_id=e.organization_id AND p.project_id=e.project_id AND p.number=e.pull_request WHERE o.id=$1 AND e.organization_id=$2`, item.Operation.ID, repository.Organization, item.Release.Source.HeadCommit).Scan(&current)
	return current, err
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
)

// MutationAuthority keeps current caller authority stable until the desired-state transaction commits.
type MutationAuthority func(context.Context, pgx.Tx) error

func mutationKey(actor, key string) (string, error) {
	if actor == "" || len(key) < 8 || len(key) > 128 {
		return "", ErrConflict
	}
	hash, err := provenance.Hash([]string{actor, key})
	return "mutation:" + hash, err
}

func mutationReceipt(ctx context.Context, tx pgx.Tx, scope v1.Scope, actor, key string, intent any) (v1.Receipt, bool, error) {
	var receipt v1.Receipt
	key, err := mutationKey(actor, key)
	if err != nil {
		return receipt, false, err
	}
	hash, err := provenance.Hash(intent)
	if err != nil {
		return receipt, false, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, scope.OrganizationID+"/"+scope.ProjectID+"/"+key); err != nil {
		return receipt, false, err
	}
	var savedHash string
	var data []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response FROM receipts WHERE organization_id=$1 AND project_id=$2 AND key=$3`, scope.OrganizationID, scope.ProjectID, key).Scan(&savedHash, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, false, reserveMutation(ctx, tx, scope.OrganizationID, actor)
	}
	if err != nil {
		return receipt, false, err
	}
	if savedHash != hash {
		return receipt, false, ErrConflict
	}
	err = json.Unmarshal(data, &receipt)
	return receipt, err == nil, err
}

func saveMutation(ctx context.Context, tx pgx.Tx, scope v1.Scope, actor, key string, intent any, receipt v1.Receipt, event string) error {
	key, err := mutationKey(actor, key)
	if err != nil {
		return err
	}
	hash, err := provenance.Hash(intent)
	if err != nil {
		return err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO receipts VALUES ($1,$2,$3,$4,$5)`, scope.OrganizationID, scope.ProjectID, key, hash, data); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(organization_id,project_id,actor,event,target,generation) VALUES ($1,$2,$3,$4,$5,$6)`, scope.OrganizationID, scope.ProjectID, actor, event, receipt.EnvironmentID, receipt.Generation)
	return err
}

func mutationPolicy(ctx context.Context, tx pgx.Tx, scope v1.Scope) (v1.PreviewPolicy, error) {
	var data []byte
	var policy v1.PreviewPolicy
	if err := tx.QueryRow(ctx, `SELECT preview_policy FROM projects WHERE organization_id=$1 AND id=$2 FOR UPDATE`, scope.OrganizationID, scope.ProjectID).Scan(&data); err != nil {
		return policy, ErrDenied
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return policy, err
	}
	return policy.Normalize()
}

// reserveMutation bounds accepted new intents; identical receipt recovery spends no budget.
func reserveMutation(ctx context.Context, tx pgx.Tx, organization, actor string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM operation_budgets WHERE organization_id=$1 AND window_start<clock_timestamp()-interval '1 day'`, organization); err != nil {
		return err
	}
	var count int
	err := tx.QueryRow(ctx, `INSERT INTO operation_budgets(organization_id,actor,window_start,count) VALUES ($1,$2,date_trunc('minute',clock_timestamp()),1)
ON CONFLICT(organization_id,actor,window_start) DO UPDATE SET count=operation_budgets.count+1 WHERE operation_budgets.count<30 RETURNING count`, organization, actor).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLimited
	}
	return err
}

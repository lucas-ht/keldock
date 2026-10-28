package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/cloud"
)

// detachCloudConsumers tombstones access/route intent in the preview deletion
// transaction. A later PR reopen cannot revive old grants or delete retained stores.
func detachCloudConsumers(ctx context.Context, tx pgx.Tx, environment string) error {
	if err := retainDatabases(ctx, tx, environment); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT i.intent FROM cloud_intents i JOIN environments e ON e.organization_id=i.organization_id AND e.project_id=i.project_id WHERE e.id=$1 AND (i.intent->'binding'->>'environmentId'=$1 OR i.intent->'domain'->'request'->>'environmentId'=$1) AND i.intent->>'action' NOT IN ('disconnect','disconnected') ORDER BY i.id FOR UPDATE OF i`, environment)
	if err != nil {
		return err
	}
	items := []cloud.Intent{}
	for rows.Next() {
		var data []byte
		var item cloud.Intent
		if err := rows.Scan(&data); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(data, &item); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		item.Generation++
		item.Action = "disconnect"
		item.State = "pending"
		item.Observation = nil
		item.UpdatedAt = time.Now().UTC()
		if item.Binding != nil {
			item.Binding.Generation = item.Generation
			item.Binding.State = "pending"
		}
		if item.Domain != nil {
			item.Domain.Generation = item.Generation
			item.Domain.State = "pending"
		}
		if err := tx.QueryRow(ctx, `UPDATE projects SET cloud_revision=cloud_revision+1 WHERE organization_id=$1 AND id=$2 RETURNING cloud_revision`, item.OrganizationID, item.ProjectID).Scan(&item.Revision); err != nil {
			return err
		}
		data, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE cloud_intents SET generation=$4,intent=$5,updated_at=now() WHERE organization_id=$1 AND project_id=$2 AND id=$3`, item.OrganizationID, item.ProjectID, item.ID, item.Generation, data); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events(organization_id,project_id,actor,event,target,generation) VALUES($1,$2,'system:preview-policy','cloud.consumer_disconnected',$3,$4)`, item.OrganizationID, item.ProjectID, item.ID, item.Generation); err != nil {
			return err
		}
	}
	return nil
}

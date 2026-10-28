package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/database"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (s *Store) backupCommand(ctx context.Context, tx pgx.Tx, p access.Principal, policy database.Policy, item database.Allocation, key, hash string, command Command) (database.Allocation, error) {
	if !p.IsAdmin() {
		return item, access.ErrDenied
	}
	destinationID := command.DestinationID
	if item.Backup != nil {
		destinationID = item.Backup.DestinationID
		if command.DestinationID != "" && command.DestinationID != destinationID {
			return item, postgres.ErrConflict
		}
	}
	d, ok := policy.Backups[destinationID]
	if !ok || d.AllocationID != item.ID || d.SourceUID != item.SourceUID || item.SourceUID == "" {
		return item, access.ErrDenied
	}
	if item.Backup == nil {
		if command.Action != "backup" {
			return item, postgres.ErrConflict
		}
		item.Backup = &database.BackupIntent{DestinationID: destinationID, SourceUID: item.SourceUID}
	}
	switch command.Action {
	case "backup":
		if item.Backup.BaseAttempt >= 10 {
			return item, postgres.ErrQuota
		}
		item.Backup.BaseAttempt++
		item.Backup.Paused = false
	case "restore":
		if item.Observation == nil || time.Since(item.Observation.ObservedAt) > 90*time.Second || item.Observation.Generation != item.Generation || item.Observation.Backup == nil || item.Observation.Backup.State != "ready" || item.Backup.RestoreAttempt >= 4 {
			return item, postgres.ErrConflict
		}
		item.Backup.RestoreAttempt++
	case "pause-backup":
		item.Backup.Paused = true
	}
	item.Generation++
	item.Observation = nil
	data, err := json.Marshal(item)
	if err != nil {
		return item, err
	}
	if _, err := tx.Exec(ctx, `UPDATE database_allocations SET generation=$4,intent=$5,observation=NULL,updated_at=now() WHERE organization_id=$1 AND project_id=$2 AND id=$3`, item.OrganizationID, item.ProjectID, item.ID, item.Generation, data); err != nil {
		return item, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO database_receipts VALUES($1,$2,$3,$4,$5)`, item.OrganizationID, item.ProjectID, key, hash, data); err != nil {
		return item, err
	}
	if err := access.RecordAudit(ctx, tx, item.OrganizationID, "user:"+p.User.ID, "database."+command.Action, item.ID); err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}

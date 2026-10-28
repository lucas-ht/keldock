package clusteridentity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucas-ht/keldock/internal/access"
)

func validToken(token, prefix string) bool {
	return len(token) == len(prefix)+43 && strings.HasPrefix(token, prefix) && strings.IndexFunc(token[len(prefix):], func(value rune) bool {
		allowed := value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '_'
		return !allowed
	}) < 0
}

// Authenticate validates durable revocation and renews one process's exclusive outbound lease.
func (repository *Repository) Authenticate(ctx context.Context, token, session, version string) (Identity, error) {
	if !validToken(token, "kda_") || len(session) < 16 || len(session) > 64 || len(version) > 64 {
		return Identity{}, access.ErrDenied
	}
	var identity Identity
	err := repository.pool.QueryRow(ctx, repository.statement(`UPDATE access_agents a SET session_id=$4,session_until=now()+interval '30 seconds',last_seen_at=now(),version=$5
FROM access_clusters c WHERE a.token_hash=$1 AND a.organization_id=$2 AND a.cluster_id=$3 AND a.revoked_at IS NULL AND a.expires_at>now()
AND c.organization_id=a.organization_id AND c.id=a.cluster_id AND c.revision=a.revision AND c.revoked_at IS NULL
AND (a.session_id=$4 OR a.session_until IS NULL OR a.session_until<now())
RETURNING a.id,a.organization_id,a.cluster_id,a.revision,a.expires_at`), access.TokenHash(token), repository.users.Organization, repository.cluster, session, version).Scan(&identity.ID, &identity.OrganizationID, &identity.ClusterID, &identity.Generation, &identity.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		var valid bool
		err := repository.pool.QueryRow(ctx, repository.statement(`SELECT EXISTS(SELECT 1 FROM access_agents a JOIN access_clusters c ON c.organization_id=a.organization_id AND c.id=a.cluster_id WHERE a.token_hash=$1 AND a.organization_id=$2 AND a.cluster_id=$3 AND a.revoked_at IS NULL AND a.expires_at>now() AND c.revoked_at IS NULL AND c.revision=a.revision)`), access.TokenHash(token), repository.users.Organization, repository.cluster).Scan(&valid)
		if err != nil {
			return identity, err
		}
		if valid {
			return identity, access.ErrSessionBusy
		}
		return identity, access.ErrDenied
	}
	return identity, err
}

// Release relinquishes only the authenticated process's lease during graceful shutdown.
func (repository *Repository) Release(ctx context.Context, token, session string) error {
	_, err := repository.pool.Exec(ctx, repository.statement(`UPDATE access_agents SET session_until=now()-interval '1 second' WHERE token_hash=$1 AND organization_id=$2 AND cluster_id=$3 AND session_id=$4 AND revoked_at IS NULL`), access.TokenHash(token), repository.users.Organization, repository.cluster, session)
	return err
}

// Rotate accepts a client-persisted replacement secret; replaying the same pending value
// recovers a lost response without storing plaintext or extending the expiry twice.
func (repository *Repository) Rotate(ctx context.Context, id, current, pending, session string) (Identity, error) {
	if !validToken(pending, "kda_") || !validToken(current, "kda_") || current == pending || len(session) < 16 || len(session) > 64 {
		return Identity{}, access.ErrInvalid
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer rollback(ctx, tx)
	if _, err := repository.lock(ctx, tx); err != nil {
		return Identity{}, err
	}
	var identity Identity
	var stored, holder string
	var lease *time.Time
	err = tx.QueryRow(ctx, repository.statement(`SELECT a.id,a.organization_id,a.cluster_id,a.revision,a.expires_at,a.token_hash,a.session_id,a.session_until
FROM access_agents a JOIN access_clusters c ON c.organization_id=a.organization_id AND c.id=a.cluster_id
WHERE a.id=$1 AND a.organization_id=$2 AND a.cluster_id=$3 AND a.revoked_at IS NULL AND a.expires_at>now() AND c.revoked_at IS NULL AND c.revision=a.revision FOR UPDATE OF a`), id, repository.users.Organization, repository.cluster).Scan(&identity.ID, &identity.OrganizationID, &identity.ClusterID, &identity.Generation, &identity.ExpiresAt, &stored, &holder, &lease)
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, access.ErrDenied
	}
	if err != nil {
		return Identity{}, err
	}
	if lease != nil && lease.After(time.Now()) && holder != session {
		return Identity{}, access.ErrSessionBusy
	}
	if stored != access.TokenHash(pending) {
		if stored != access.TokenHash(current) {
			return Identity{}, access.ErrDenied
		}
		if err := tx.QueryRow(ctx, repository.statement(`UPDATE access_agents SET token_hash=$2,expires_at=now()+interval '30 days',rotated_at=now() WHERE id=$1 RETURNING expires_at`), id, access.TokenHash(pending)).Scan(&identity.ExpiresAt); err != nil {
			return Identity{}, err
		}
		if err := access.RecordAudit(ctx, tx, identity.OrganizationID, "agent:"+id, "agent.rotated", identity.ClusterID); err != nil {
			return Identity{}, err
		}
	}
	identity.Token = pending
	return identity, tx.Commit(ctx)
}

// Information is safe connection metadata for an already authorized inventory read.
func (repository *Repository) Information(ctx context.Context) (Cluster, error) {
	var result Cluster
	err := repository.pool.QueryRow(ctx, repository.statement(`SELECT c.id,c.name,c.revision,c.revoked_at,coalesce(a.id,''),coalesce(a.version,''),a.expires_at,a.last_seen_at
FROM access_clusters c LEFT JOIN access_agents a ON a.organization_id=c.organization_id AND a.cluster_id=c.id AND a.revoked_at IS NULL
WHERE c.organization_id=$1 AND c.id=$2`), repository.users.Organization, repository.cluster).Scan(&result.ID, &result.Name, &result.Revision, &result.RevokedAt, &result.AgentID, &result.Version, &result.ExpiresAt, &result.LastSeenAt)
	if err != nil {
		return result, err
	}
	result.State = "unenrolled"
	switch {
	case result.RevokedAt != nil:
		result.State = "revoked"
	case result.ExpiresAt != nil && result.ExpiresAt.Before(time.Now()):
		result.State = "expired"
	case result.LastSeenAt != nil && time.Since(*result.LastSeenAt) < 30*time.Second:
		result.State = "connected"
	case result.AgentID != "":
		result.State = "offline"
	}
	return result, nil
}

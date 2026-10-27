package clusteridentity

import (
	"context"

	"github.com/lucas-ht/keldock/internal/access"
)

// CreateEnrollment issues a ten-minute grant and invalidates older unconsumed grants.
// The existing agent remains active until replacement enrollment or explicit revocation.
func (repository *Repository) CreateEnrollment(ctx context.Context, principal access.Principal) (Enrollment, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	defer rollback(ctx, tx)
	if err := repository.administrator(ctx, tx, principal); err != nil {
		return Enrollment{}, err
	}
	revision, err := repository.lock(ctx, tx)
	if err != nil {
		return Enrollment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_enrollments SET consumed_at=now() WHERE organization_id=$1 AND cluster_id=$2 AND consumed_at IS NULL`, repository.users.Organization, repository.cluster); err != nil {
		return Enrollment{}, err
	}
	grant := Enrollment{ClusterID: repository.cluster, Token: access.NewToken("kde_")}
	err = tx.QueryRow(ctx, `INSERT INTO access_enrollments (token_hash,organization_id,cluster_id,revision,expires_at,created_by) VALUES ($1,$2,$3,$4,now()+interval '10 minutes',$5) RETURNING expires_at`, access.TokenHash(grant.Token), repository.users.Organization, repository.cluster, revision, principal.User.ID).Scan(&grant.ExpiresAt)
	if err != nil {
		return Enrollment{}, err
	}
	if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+principal.User.ID, "cluster.enrollment_created", repository.cluster); err != nil {
		return Enrollment{}, err
	}
	return grant, tx.Commit(ctx)
}

// Exchange consumes a grant once, replaces older agent authority and returns one secret.
func (repository *Repository) Exchange(ctx context.Context, cluster, token string) (Identity, error) {
	if cluster != repository.cluster || !validToken(token, "kde_") {
		return Identity{}, access.ErrDenied
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer rollback(ctx, tx)
	revision, err := repository.lock(ctx, tx)
	if err != nil {
		return Identity{}, err
	}
	var grantRevision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM access_enrollments WHERE token_hash=$1 AND organization_id=$2 AND cluster_id=$3 AND expires_at>now() AND consumed_at IS NULL FOR UPDATE`, access.TokenHash(token), repository.users.Organization, cluster).Scan(&grantRevision)
	if err != nil || grantRevision != revision {
		return Identity{}, access.ErrDenied
	}
	if _, err := tx.Exec(ctx, `UPDATE access_enrollments SET consumed_at=now() WHERE organization_id=$1 AND cluster_id=$2 AND consumed_at IS NULL`, repository.users.Organization, cluster); err != nil {
		return Identity{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_agents SET revoked_at=now() WHERE organization_id=$1 AND cluster_id=$2 AND revoked_at IS NULL`, repository.users.Organization, cluster); err != nil {
		return Identity{}, err
	}
	revision++
	if _, err := tx.Exec(ctx, `UPDATE access_clusters SET revision=$3,revoked_at=NULL WHERE organization_id=$1 AND id=$2`, repository.users.Organization, cluster, revision); err != nil {
		return Identity{}, err
	}
	identity := Identity{ID: access.NewID("agt"), OrganizationID: repository.users.Organization, ClusterID: cluster, Generation: revision, Token: access.NewToken("kda_")}
	err = tx.QueryRow(ctx, `INSERT INTO access_agents (id,organization_id,cluster_id,revision,token_hash,expires_at) VALUES ($1,$2,$3,$4,$5,now()+interval '30 days') RETURNING expires_at`, identity.ID, identity.OrganizationID, cluster, revision, access.TokenHash(identity.Token)).Scan(&identity.ExpiresAt)
	if err != nil {
		return Identity{}, err
	}
	if err := access.RecordAudit(ctx, tx, identity.OrganizationID, "agent:"+identity.ID, "cluster.enrolled", cluster); err != nil {
		return Identity{}, err
	}
	return identity, tx.Commit(ctx)
}

// Revoke invalidates agent credentials and outstanding grants without deleting workloads.
func (repository *Repository) Revoke(ctx context.Context, principal access.Principal) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if err := repository.administrator(ctx, tx, principal); err != nil {
		return err
	}
	if _, err := repository.lock(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_clusters SET revision=revision+1,revoked_at=now() WHERE organization_id=$1 AND id=$2`, repository.users.Organization, repository.cluster); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_agents SET revoked_at=now() WHERE organization_id=$1 AND cluster_id=$2 AND revoked_at IS NULL`, repository.users.Organization, repository.cluster); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE access_enrollments SET consumed_at=now() WHERE organization_id=$1 AND cluster_id=$2 AND consumed_at IS NULL`, repository.users.Organization, repository.cluster); err != nil {
		return err
	}
	if err := access.RecordAudit(ctx, tx, repository.users.Organization, "user:"+principal.User.ID, "cluster.revoked", repository.cluster); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

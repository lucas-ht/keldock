package store

import (
	"context"

	"github.com/lucas-ht/keldock/internal/access"
)

// FeedbackCounts distinguishes pending reports from reports currently failing delivery.
func (repository *Repository) FeedbackCounts(ctx context.Context) (pending, failed int, err error) {
	err = repository.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE sent_hash=''),count(*) FILTER (WHERE error<>'') FROM github_feedback WHERE organization_id=$1`, repository.Organization).Scan(&pending, &failed)
	return
}

// RetryWork repairs exhausted webhook attempts and failed reporting without admitting releases.
func (repository *Repository) RetryWork(ctx context.Context, principal access.Principal, expected int64) error {
	return repository.AdminAction(ctx, principal, expected, "github.work_retried", func() error {
		if _, err := repository.pool.Exec(ctx, `UPDATE github_events SET attempt=0,available_at=now(),error='' WHERE organization_id=$1 AND processed_at IS NULL AND (lease_until IS NULL OR lease_until<now())`, repository.Organization); err != nil {
			return err
		}
		_, err := repository.pool.Exec(ctx, `UPDATE github_feedback SET attempt=0,available_at=now() WHERE organization_id=$1 AND error<>'' AND (lease_until IS NULL OR lease_until<now())`, repository.Organization)
		return err
	})
}

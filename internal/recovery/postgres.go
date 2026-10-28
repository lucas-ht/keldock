package recovery

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func postgresTool(ctx context.Context, dsn, command string, args ...string) ([]byte, error) {
	process := exec.CommandContext(ctx, command, args...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PG") {
			process.Env = append(process.Env, value)
		}
	}
	if dsn != "" {
		parsed, err := url.Parse(dsn)
		if err != nil || parsed.User == nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
			return nil, fmt.Errorf("recovery requires a PostgreSQL URI")
		}
		password, _ := parsed.User.Password()
		port := parsed.Port()
		if port == "" {
			port = "5432"
		}
		process.Env = append(process.Env, "PGHOST="+parsed.Hostname(), "PGPORT="+port, "PGUSER="+parsed.User.Username(), "PGPASSWORD="+password, "PGDATABASE="+strings.TrimPrefix(parsed.Path, "/"))
		for key, env := range map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY"} {
			if value := parsed.Query().Get(key); value != "" {
				process.Env = append(process.Env, env+"="+value)
			}
		}
	}
	process.Env = append(process.Env, "PGAPPNAME=keldock-recovery", "PGCONNECT_TIMEOUT=10")
	output, err := process.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed; verify the PostgreSQL 18 client and target connectivity", command)
	}
	return output, nil
}

func prepareTarget(ctx context.Context, s *postgres.Store, manifest Manifest) error {
	var tables int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename!='recovery_state'`).Scan(&tables); err != nil {
		return err
	}
	if tables != 0 {
		return fmt.Errorf("restore requires an empty independent target database")
	}
	// Install the fence BEFORE pg_restore can commit any recovered schema. The
	// restore list excludes this table, so an auto-restarting server cannot race it.
	if _, err := s.Pool().Exec(ctx, `CREATE TABLE IF NOT EXISTS recovery_state(organization_id text PRIMARY KEY,fenced boolean NOT NULL DEFAULT true,snapshot_id text NOT NULL,restored_at timestamptz NOT NULL DEFAULT now(),resumed_at timestamptz)`); err != nil {
		return err
	}
	_, err := s.Pool().Exec(ctx, `INSERT INTO recovery_state(organization_id,snapshot_id) VALUES($1,$2) ON CONFLICT(organization_id) DO UPDATE SET fenced=true,snapshot_id=EXCLUDED.snapshot_id`, manifest.Organization, manifest.ID)
	return err
}

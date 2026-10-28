package recovery

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// Backup saves database state, private configuration and all referenced OCI graphs.
// It never uploads secrets; operators choose encryption and off-machine storage.
func Backup(ctx context.Context, config Config, directory string) (Manifest, error) {
	manifest := Manifest{Version: 1, ID: "snapshot-" + strings.ToLower(rand.Text()), Organization: config.Organization, CreatedAt: time.Now().UTC()}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return manifest, fmt.Errorf("snapshot destination must be a new private directory")
	}
	s, err := postgres.Open(ctx, config.DatabaseURL)
	if err != nil {
		return manifest, err
	}
	defer s.Close()
	var foreign int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM(SELECT organization_id FROM projects UNION SELECT organization_id FROM access_users) owners WHERE organization_id<>$1`, config.Organization).Scan(&foreign); err != nil {
		return manifest, err
	}
	if foreign != 0 {
		return manifest, fmt.Errorf("the supported snapshot profile requires a dedicated one-organization control-plane database")
	}
	tx, err := s.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return manifest, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var snapshot string
	if err := tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		return manifest, err
	}
	manifest.Artifacts, err = artifactInventory(ctx, tx, config.Organization)
	if err != nil {
		return manifest, err
	}
	if err := os.Mkdir(filepath.Join(directory, "private"), 0o700); err != nil {
		return manifest, err
	}
	hashes := map[string]string{}
	for name, path := range config.Files {
		if !filepath.IsLocal(name) || filepath.Base(name) != name {
			return manifest, fmt.Errorf("invalid snapshot private filename")
		}
		if err := privateFile(path, filepath.Join(directory, "private", name)); err != nil {
			return manifest, err
		}
		hash, err := checksum(path)
		if err != nil {
			return manifest, err
		}
		hashes[path] = hash
	}
	if _, err := postgresTool(ctx, config.DatabaseURL, "pg_dump", "--snapshot", snapshot, "--format=custom", "--no-owner", "--no-privileges", "--file", filepath.Join(directory, "database.dump")); err != nil {
		return manifest, err
	}
	if err := os.Chmod(filepath.Join(directory, "database.dump"), 0o600); err != nil {
		return manifest, err
	}
	if err := exportArtifacts(ctx, config, manifest.Artifacts, filepath.Join(directory, "oci")); err != nil {
		return manifest, err
	}
	for path, previous := range hashes {
		current, err := checksum(path)
		if err != nil || previous != current {
			return manifest, fmt.Errorf("installation credentials changed while snapshotting; retry")
		}
	}
	manifest.Files, err = inventory(directory)
	if err != nil {
		return manifest, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return manifest, err
	}
	return manifest, os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o600)
}

// Restore requires an empty independent database and installs its recovery fence first.
// It never overwrites installation files or resumes any agent/server automatically.
func Restore(ctx context.Context, config Config, directory string) error {
	manifest, err := Verify(directory)
	if err != nil {
		return err
	}
	if manifest.Organization != config.Organization {
		return fmt.Errorf("snapshot organization does not match target")
	}
	s, err := postgres.Open(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := prepareTarget(ctx, s, manifest); err != nil {
		return err
	}
	dump := filepath.Join(directory, "database.dump")
	listing, err := postgresTool(ctx, "", "pg_restore", "--list", dump)
	if err != nil {
		return err
	}
	lines := []string{}
	for _, line := range strings.Split(string(listing), "\n") {
		if !strings.Contains(line, "recovery_state") {
			lines = append(lines, line)
		}
	}
	list, err := os.CreateTemp("", "keldock-restore-*.list")
	if err != nil {
		return err
	}
	path := list.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := list.WriteString(strings.Join(lines, "\n")); err != nil {
		_ = list.Close()
		return err
	}
	if err := list.Close(); err != nil {
		return err
	}
	parsed, err := url.Parse(config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("invalid recovery database URI")
	}
	if _, err := postgresTool(ctx, config.DatabaseURL, "pg_restore", "--dbname", strings.TrimPrefix(parsed.Path, "/"), "--no-owner", "--no-privileges", "--single-transaction", "--exit-on-error", "--use-list", path, dump); err != nil {
		return err
	}
	return s.FenceRecovery(ctx, config.Organization, manifest.ID)
}

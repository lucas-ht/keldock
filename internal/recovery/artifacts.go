package recovery

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func artifactInventory(ctx context.Context, tx pgx.Tx, organization string) ([]Artifact, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT d.bundle,e.project_id,'bundle' FROM deployments d JOIN environments e ON e.id=d.environment_id WHERE e.organization_id=$1 AND d.bundle!='' UNION SELECT DISTINCT images.value,r.project_id,'image' FROM releases r CROSS JOIN LATERAL jsonb_each_text(r.input->'images') images WHERE r.organization_id=$1 ORDER BY 1,2,3`, organization)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Artifact{}
	for rows.Next() {
		var item Artifact
		if err := rows.Scan(&item.Reference, &item.ProjectID, &item.Kind); err != nil {
			return nil, err
		}
		item.Tag = fmt.Sprintf("artifact-%x", sha256.Sum256([]byte(item.Reference)))
		result = append(result, item)
		if len(result) > 4096 {
			return nil, fmt.Errorf("snapshot artifact limit exceeded")
		}
	}
	return result, rows.Err()
}

func repository(config Config, item Artifact) (*remote.Repository, string, error) {
	name, digest, err := artifacts.SplitReference(item.Reference)
	if err != nil {
		return nil, "", err
	}
	alias := strings.SplitN(config.Registry.PullRepository, "/", 2)[0]
	host := strings.SplitN(config.Registry.PushRepository, "/", 2)[0]
	transportName := name
	if strings.HasPrefix(name, alias+"/") {
		transportName = host + strings.TrimPrefix(name, alias)
	}
	repo, err := remote.NewRepository(transportName)
	if err != nil {
		return nil, "", err
	}
	repo.PlainHTTP = config.Registry.PlainHTTP && strings.HasPrefix(name, alias+"/")
	credential := auth.Credential{}
	if name == config.Registry.PullRepository {
		credential = auth.Credential{Username: config.Registry.Username, Password: config.Registry.Password}
	}
	for _, reader := range config.Pulls.Workloads[item.ProjectID] {
		if reader.Repository == name {
			credential = auth.Credential{Username: reader.Username, Password: reader.Password}
		}
	}
	repo.Client = &auth.Client{Client: &http.Client{Timeout: time.Minute}, Cache: auth.NewCache(), Credential: auth.StaticCredential(repo.Reference.Registry, credential)}
	return repo, digest, nil
}

func exportArtifacts(ctx context.Context, config Config, items []Artifact, directory string) error {
	layout, err := oci.New(directory)
	if err != nil {
		return err
	}
	for _, item := range items {
		repo, digest, err := repository(config, item)
		if err != nil {
			return err
		}
		actual, err := oras.Copy(ctx, repo, digest, layout, item.Tag, oras.DefaultCopyOptions)
		if err != nil {
			return fmt.Errorf("snapshot artifact unavailable for project %s", item.ProjectID)
		}
		if actual.Digest.String() != digest {
			return fmt.Errorf("snapshot artifact digest mismatch")
		}
	}
	return nil
}

// RestoreBundles republishes only Keldock's own bundle repository, from verified bytes.
// Workload image graphs remain in the portable OCI layout for their registry owner.
func RestoreBundles(ctx context.Context, config Config, directory string) error {
	manifest, err := Verify(directory)
	if err != nil {
		return err
	}
	layout, err := oci.New(directory + "/oci")
	if err != nil {
		return err
	}
	for _, item := range manifest.Artifacts {
		if item.Kind != "bundle" {
			continue
		}
		if !strings.HasPrefix(item.Reference, config.Registry.PullRepository+"@") {
			return fmt.Errorf("bundle restore target changed")
		}
		repo, digest, err := repository(config, item)
		if err != nil {
			return err
		}
		actual, err := oras.Copy(ctx, layout, item.Tag, repo, "bundle-"+strings.TrimPrefix(digest, "sha256:"), oras.DefaultCopyOptions)
		if err != nil {
			return fmt.Errorf("bundle registry restore failed")
		}
		if actual.Digest.String() != digest {
			return fmt.Errorf("restored bundle digest changed")
		}
	}
	return nil
}

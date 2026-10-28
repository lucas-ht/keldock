package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

type credentialState struct {
	Identity clusteridentity.Identity `json:"identity"`
	Pending  string                   `json:"pending,omitempty"`
	path     string
}

// Enroll consumes a short-lived grant and saves its replacement credential in a private file.
func Enroll(ctx context.Context, endpoint, cluster, token, path string, development bool) error {
	return enroll(ctx, endpoint, cluster, token, path, development, false, false)
}

// EnrollCloud exchanges only a cloud enrollment and stores its independent credential.
func EnrollCloud(ctx context.Context, endpoint, cluster, token, path string, development bool) error {
	return enroll(ctx, endpoint, cluster, token, path, development, true, false)
}

// EnrollDatabase exchanges a separate database provisioning grant.
func EnrollDatabase(ctx context.Context, endpoint, cluster, token, path string, development bool) error {
	return enroll(ctx, endpoint, cluster, token, path, development, false, true)
}

func enroll(ctx context.Context, endpoint, cluster, token, path string, development, cloud, database bool) error {
	if !v1.ValidName(cluster) || path == "" {
		return fmt.Errorf("cluster and credential file required")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("credential destination must not exist")
	}
	client, err := NewClient(endpoint, token, development)
	if err != nil {
		return err
	}
	client.cloud = cloud
	client.database = database
	var identity clusteridentity.Identity
	if err := client.request(ctx, "POST", "/v1/agent/enroll", map[string]string{"clusterId": cluster, "token": token}, &identity); err != nil {
		return err
	}
	state := credentialState{Identity: identity, path: path}
	return state.save()
}

// ClientFromFile loads a scoped enrollment, including a pending rotation after interruption.
func ClientFromFile(endpoint, path string, development bool) (*Client, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 8192 {
		return nil, fmt.Errorf("agent credentials require a bounded owner-only regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state credentialState
	if err := v1.DecodeJSON(data, &state); err != nil {
		return nil, err
	}
	if state.Identity.ID == "" || !v1.ValidName(state.Identity.ClusterID) || state.Identity.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("invalid enrolled agent identity")
	}
	state.path = path
	client, err := NewClient(endpoint, state.Identity.Token, development)
	if err != nil {
		return nil, err
	}
	client.credentials = &state
	return client, nil
}

func (state *credentialState) save() error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(state.path), ".keldock-credential-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, state.path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(state.path))
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

// RefreshCredential persists the next secret before exchange, making lost responses recoverable.
func (client *Client) RefreshCredential(ctx context.Context) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	state := client.credentials
	if state == nil || (state.Pending == "" && time.Until(state.Identity.ExpiresAt) > 7*24*time.Hour) {
		return nil
	}
	if state.Pending == "" {
		state.Pending = access.NewToken("kda_")
		if err := state.save(); err != nil {
			return err
		}
	}
	var identity clusteridentity.Identity
	if err := client.requestLocked(ctx, "POST", "/v1/agent/rotate", map[string]string{"id": state.Identity.ID, "token": state.Pending}, &identity); err != nil {
		return err
	}
	replacement := *state
	replacement.Identity = identity
	replacement.Pending = ""
	if err := replacement.save(); err != nil {
		return err
	}
	*state = replacement
	client.token = identity.Token
	return nil
}

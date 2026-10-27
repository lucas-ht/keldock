//go:build integration

package runtime_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/admission"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane"
	"github.com/lucas-ht/keldock/internal/provenance"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

type apiFixture struct {
	api      *controlplane.API
	server   *httptest.Server
	issuer   *httptest.Server
	identity *rsa.PrivateKey
	signing  ed25519.PrivateKey
	scope    v1.Scope
	store    *postgres.Store
}

type operationStatus struct {
	APIVersion string          `json:"apiVersion"`
	Operation  v1.Operation    `json:"operation"`
	Report     *v1.Observation `json:"observation"`
	Age        float64         `json:"observationAgeSeconds"`
	Stale      bool            `json:"stale"`
}

func newAPI(t *testing.T) *apiFixture {
	t.Helper()
	store, ctx := database(t)
	scope, _ := scopedInput(t, store, ctx)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &apiFixture{store: store, scope: scope, identity: key}
	fixture.issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response any
		if r.URL.Path == "/.well-known/openid-configuration" {
			response = map[string]any{"issuer": fixture.issuer.URL, "jwks_uri": fixture.issuer.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}}
		} else {
			response = map[string]any{"keys": []map[string]any{{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "fixture", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(fixture.issuer.Close)
	public, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture.signing = signing
	verifier, err := admission.NewGitHub(ctx, fixture.issuer.URL, "keldock:fixture", []admission.RepositoryPolicy{{Scope: scope, RepositoryID: 42, OwnerID: 24, WorkflowPath: ".github/workflows/build.yaml", PushRef: "refs/heads/main", Keys: map[string]ed25519.PublicKey{"fixture": public}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	project := v1.Project{Scope: scope, Name: "Sample", RepositoryID: 42, ClusterID: "keldock-m1"}
	fixture.api = &controlplane.API{Store: store, GitHub: verifier, Organization: scope.OrganizationID, Cluster: "keldock-m1", Projects: map[string]v1.Project{"sample": project}, RepositoryOwners: map[int64]int64{42: 24}, OperatorToken: strings.Repeat("o", 48), AgentToken: strings.Repeat("a", 48), WebhookSecret: bytes.Repeat([]byte("w"), 48), Domain: "keldock.test"}
	fixture.server = httptest.NewServer(fixture.api.Handler())
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *apiFixture) token(t *testing.T, source v1.Source) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"fixture","typ":"JWT"}`))
	claims := map[string]any{"iss": fixture.issuer.URL, "aud": "keldock:fixture", "sub": "repo:" + source.Repository, "exp": time.Now().Add(5 * time.Minute).Unix(), "repository_id": strconv.FormatInt(source.RepositoryID, 10), "repository_owner_id": strconv.FormatInt(source.OwnerID, 10), "repository": source.Repository, "sha": source.Commit, "ref": source.Ref, "event_name": source.Event, "workflow_ref": source.WorkflowRef, "run_id": strconv.FormatInt(source.RunID, 10), "run_attempt": strconv.FormatInt(source.RunAttempt, 10)}
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	message := header + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, fixture.identity, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func (fixture *apiFixture) sign(t *testing.T, input v1.SubmitRelease) v1.SubmitRelease {
	t.Helper()
	statement, err := provenance.NewStatement(input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := provenance.Sign(body, "fixture", fixture.signing)
	if err != nil {
		t.Fatal(err)
	}
	input.Provenance, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func (fixture *apiFixture) call(t *testing.T, method, path string, headers map[string]string, input, output any, expected int) {
	t.Helper()
	body, ok := input.([]byte)
	if !ok && input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequestWithContext(context.Background(), method, fixture.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected {
		t.Fatalf("%s %s: wanted %d, got %d: %.1024s", method, path, expected, response.StatusCode, data)
	}
	if output != nil {
		if err := v1.DecodeJSON(data, output); err != nil {
			t.Fatal(err)
		}
	}
}

func (fixture *apiFixture) submit(t *testing.T, input v1.SubmitRelease, key string, expected int) v1.Receipt {
	t.Helper()
	var receipt v1.Receipt
	var output any
	if expected == http.StatusAccepted {
		output = &receipt
	}
	fixture.call(t, http.MethodPost, "/v1/releases", map[string]string{"Authorization": "Bearer " + fixture.token(t, input.Source), "Idempotency-Key": key}, input, output, expected)
	return receipt
}

func (fixture *apiFixture) webhook(t *testing.T, input v1.SubmitRelease, action string, updated time.Time) {
	t.Helper()
	state := "open"
	if action == "closed" {
		state = "closed"
	}
	body, err := json.Marshal(map[string]any{"action": action, "number": input.Source.PullRequest, "repository": map[string]any{"id": 42, "owner": map[string]any{"id": 24}}, "pull_request": map[string]any{"number": input.Source.PullRequest, "state": state, "updated_at": updated, "head": map[string]any{"sha": input.Source.HeadCommit, "repo": map[string]any{"id": 42}}}})
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, fixture.api.WebhookSecret)
	if _, err := mac.Write(body); err != nil {
		t.Fatal(err)
	}
	fixture.call(t, http.MethodPost, "/v1/github/webhooks", map[string]string{"X-GitHub-Event": "pull_request", "X-GitHub-Delivery": strings.ToLower(rand.Text()), "X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))}, body, nil, http.StatusAccepted)
}

func startCLI(t *testing.T, ctx context.Context, binary string, args ...string) {
	t.Helper()
	log, err := os.OpenFile(filepath.Join(t.TempDir(), "process.log"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdout, command.Stderr = log, log
	command.WaitDelay = 3 * time.Second
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	t.Cleanup(func() {
		if err := command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error(err)
		}
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("CLI process failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			<-finished
			t.Error("CLI process did not stop gracefully")
		}
		if err := log.Close(); err != nil {
			t.Error(err)
		}
		if t.Failed() {
			data, err := os.ReadFile(log.Name())
			if err == nil {
				t.Logf("CLI diagnostic: %.2048s", data)
			}
		}
	})
}

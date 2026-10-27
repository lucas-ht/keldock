package admission

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
)

func signedToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test","typ":"JWT"}`))
	data, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	message := header + "." + base64.RawURLEncoding.EncodeToString(data)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestGitHubOIDCAndProvenance(t *testing.T) {
	ctx := context.Background()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		var response any
		if request.URL.Path == "/.well-known/openid-configuration" {
			response = map[string]any{"issuer": issuer.URL, "jwks_uri": issuer.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}}
		} else {
			response = map[string]any{"keys": []map[string]any{{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}
		}
		if err := json.NewEncoder(writer).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer issuer.Close()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	policy := RepositoryPolicy{Scope: v1.Scope{OrganizationID: "local", ProjectID: "sample"}, RepositoryID: 42, OwnerID: 24, WorkflowPath: ".github/workflows/build.yaml", PushRef: "refs/heads/main", Keys: map[string]ed25519.PublicKey{"build-key": public}}
	verifier, err := NewGitHub(ctx, issuer.URL, "keldock:local", []RepositoryPolicy{policy}, true)
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{"iss": issuer.URL, "aud": "keldock:local", "sub": "repo:example/sample:ref:refs/heads/main", "exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(), "repository_id": "42", "repository_owner_id": "24", "repository": "example/sample", "sha": strings.Repeat("a", 40), "ref": "refs/heads/main", "event_name": "push", "workflow_ref": "example/sample/.github/workflows/build.yaml@refs/heads/main", "run_id": "10", "run_attempt": "1"}
	principal, err := verifier.Authenticate(ctx, signedToken(t, key, claims))
	if err != nil || principal.Scope != policy.Scope {
		t.Fatalf("identity: %+v %v", principal, err)
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: "sample", Source: principal.Source, Definition: v1.Application{APIVersion: v1.APIVersion, Name: "sample", Components: map[string]v1.Component{"web": {Kind: "http", Size: "small", Port: 8080, Probe: &v1.Probe{Path: "/ready"}}}}, Images: map[string]string{"web": "registry.example/sample@sha256:" + strings.Repeat("b", 64)}}
	statement, err := provenance.NewStatement(input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := provenance.Sign(body, "build-key", private)
	if err != nil {
		t.Fatal(err)
	}
	input.Provenance, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.VerifyRelease(principal, input); err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.Images = map[string]string{"web": "registry.example/sample@sha256:" + strings.Repeat("c", 64)}
	if err := verifier.VerifyRelease(principal, changed); !errors.Is(err, ErrDenied) {
		t.Fatalf("valid OIDC bypassed artifact origin: %v", err)
	}
	if err := verifier.VerifyRelease(Principal{Scope: policy.Scope, Source: principal.Source}, input); !errors.Is(err, ErrDenied) {
		t.Fatal("forged principal accepted")
	}
	for _, test := range []struct {
		name, field string
		value       any
	}{
		{"audience", "aud", "other"}, {"repository", "repository_id", "99"}, {"owner transfer", "repository_owner_id", "99"},
		{"workflow", "workflow_ref", "example/sample/.github/workflows/other.yaml@refs/heads/main"},
		{"unapproved ref", "ref", "refs/heads/untrusted"}, {"fork target event", "event_name", "pull_request_target"},
		{"reusable workflow", "job_workflow_ref", "other/workflow@main"}, {"expired", "exp", time.Now().Add(-time.Hour).Unix()},
	} {
		t.Run(test.name, func(t *testing.T) {
			copied := map[string]any{}
			for field, value := range claims {
				copied[field] = value
			}
			copied[test.field] = test.value
			if _, err := verifier.Authenticate(ctx, signedToken(t, key, copied)); !errors.Is(err, ErrDenied) {
				t.Fatalf("untrusted identity accepted: %v", err)
			}
		})
	}
	if _, err := NewGitHub(ctx, issuer.URL, "keldock:local", []RepositoryPolicy{policy}, false); err == nil {
		t.Fatal("insecure issuer enabled without explicit local profile")
	}
}

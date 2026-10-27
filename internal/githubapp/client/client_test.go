package client_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucas-ht/keldock/internal/githubapp/client"
	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
)

func testApp(t *testing.T) (credentials.App, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return credentials.App{ID: 42, ClientID: "client-42", Slug: "keldock-test", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), WebhookSecret: strings.Repeat("a", 32)}, key
}

func TestJWTClaimsSignatureAndOriginBoundary(t *testing.T) {
	app, key := testApp(t)
	api, err := client.New(app, client.Options{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	token, err := api.JWT(now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("invalid JWT")
	}
	var claims struct {
		Issuer string `json:"iss"`
		Issued int64  `json:"iat"`
		Expiry int64  `json:"exp"`
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Issuer != app.ClientID || claims.Issued > now.Unix() || claims.Expiry > now.Add(10*time.Minute).Unix() {
		t.Fatal("invalid App identity claims")
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], signature); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"http://api.github.com", "https://example.com", "https://api.github.com/path", "https://user:password@api.github.com", "http://127.0.0.1:1234"} {
		t.Run(base, func(t *testing.T) {
			if _, err := client.New(app, client.Options{BaseURL: base}); err == nil {
				t.Fatal("unapproved API origin accepted")
			}
		})
	}
}

func TestRequestBoundsRedirectsAndRateLimit(t *testing.T) {
	app, _ := testApp(t)
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("X-GitHub-Api-Version") != client.APIVersion {
			t.Error("missing pinned authenticated request")
		}
		switch r.URL.Path {
		case "/app":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"slug":"keldock-test"}`))
		case "/rate":
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(429)
			_, _ = w.Write([]byte("secret response must never escape"))
		case "/redirect":
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", 2*1024*1024+1)))
		}
	}))
	defer server.Close()
	api, err := client.New(app, client.Options{BaseURL: server.URL, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.App(context.Background()); err != nil {
		t.Fatal(err)
	}
	err = api.AppRequest(context.Background(), "GET", "/rate", nil, nil)
	var failure *client.Error
	if !errors.As(err, &failure) || failure.RetryAfter != 120*time.Second || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe rate-limit failure: %v", err)
	}
	if err := api.AppRequest(context.Background(), "GET", "/redirect", nil, nil); err == nil || redirected {
		t.Fatal("credential-bearing redirect was followed")
	}
	var value any
	if err := api.AppRequest(context.Background(), "GET", "/large", nil, &value); err == nil {
		t.Fatal("oversized response accepted")
	}
	for _, name := range []string{"owner/../repository", "owner/repository?token=value", "https://github.com/owner/repo", "owner/.."} {
		if _, err := client.RepositoryPath(name); err == nil {
			t.Fatal("unsafe repository path accepted")
		}
	}
}

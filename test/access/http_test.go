//go:build integration

package access_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
)

func TestHTTPAuthenticationCSRFAndAccountManagement(t *testing.T) {
	fixture := newIdentityHTTP(t)
	fixture.call(t, fixture.client, http.MethodGet, "/v1/auth/me", nil, nil, nil, http.StatusUnauthorized)
	fixture.call(t, fixture.client, http.MethodGet, "/v1/users", nil, nil, nil, http.StatusUnauthorized)
	fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/login", map[string]string{"Origin": "https://attacker.example"}, map[string]string{"email": "admin@example.test", "password": "a private local test password"}, nil, http.StatusForbidden)
	user := fixture.activate(t)
	var me struct {
		User access.User `json:"user"`
		CSRF string      `json:"csrfToken"`
	}
	response := fixture.call(t, fixture.client, http.MethodGet, "/v1/auth/me", nil, nil, &me, http.StatusOK)
	if me.User.ID != user.ID || me.CSRF != fixture.csrf || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("session identity or cache policy incorrect")
	}
	for _, cookie := range fixture.client.Jar.Cookies(response.Request.URL) {
		if cookie.Name == fixture.service.CookieName() && (strings.Contains(cookie.Value, fixture.csrf) || cookie.Value == "") {
			t.Fatal("session and anti-forgery proof were conflated")
		}
	}
	fixture.call(t, fixture.client, http.MethodPost, "/v1/users", map[string]string{"X-Keldock-CSRF": ""}, map[string]any{"name": "Viewer", "email": "viewer@example.test", "role": "viewer", "projects": []string{"sample"}}, nil, http.StatusForbidden)
	var invited struct {
		User access.User `json:"user"`
		URL  string      `json:"activationUrl"`
	}
	fixture.call(t, fixture.client, http.MethodPost, "/v1/users", nil, map[string]any{"name": "Viewer", "email": "viewer@example.test", "role": "viewer", "projects": []string{"sample"}}, &invited, http.StatusCreated)
	if !strings.Contains(invited.URL, "/activate#kdi_") {
		t.Fatal("invitation secret was put in a server-visible query")
	}
	fixture.call(t, fixture.client, http.MethodPatch, "/v1/users/"+user.ID, nil, access.UserUpdate{Name: user.Name, Role: "viewer", Status: "active", Version: user.Version}, nil, http.StatusConflict)
	fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/password", nil, map[string]string{"currentPassword": "wrong password", "password": "a changed private test password"}, nil, http.StatusBadRequest)
	var changed struct {
		CSRF string `json:"csrfToken"`
	}
	fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/password", nil, map[string]string{"currentPassword": "a private local test password", "password": "a changed private test password"}, &changed, http.StatusOK)
	if changed.CSRF == fixture.csrf {
		t.Fatal("password change did not rotate browser authority")
	}
	fixture.csrf = changed.CSRF
	fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/logout", nil, nil, nil, http.StatusOK)
	fixture.call(t, fixture.client, http.MethodGet, "/v1/auth/me", nil, nil, nil, http.StatusUnauthorized)
	fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/login", nil, map[string]string{"email": "admin@example.test", "password": "a private local test password"}, nil, http.StatusUnauthorized)
	login := fixture.call(t, fixture.client, http.MethodPost, "/v1/auth/login", nil, map[string]string{"email": "admin@example.test", "password": "a changed private test password"}, &changed, http.StatusOK)
	var found bool
	for _, cookie := range login.Cookies() {
		if cookie.Name == fixture.service.CookieName() {
			found = true
			if !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 43200 {
				t.Fatal("browser cookie protection missing")
			}
		}
	}
	if !found {
		t.Fatal("successful login did not set a session cookie")
	}
}

func TestHTTPClusterEnrollmentAndMCPSettingsAreAdministrative(t *testing.T) {
	fixture := newIdentityHTTP(t)
	fixture.activate(t)
	var enrollment struct {
		Grant struct {
			Cluster string `json:"clusterId"`
			Token   string `json:"token"`
		} `json:"enrollment"`
	}
	fixture.call(t, fixture.client, http.MethodPost, "/v1/clusters/keldock-m1/enrollment", nil, nil, &enrollment, http.StatusCreated)
	var identity struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	anonymous := &http.Client{}
	fixture.call(t, anonymous, http.MethodPost, "/v1/agent/enroll", nil, map[string]string{"clusterId": enrollment.Grant.Cluster, "token": enrollment.Grant.Token}, &identity, http.StatusCreated)
	headers := map[string]string{"Authorization": "Bearer " + identity.Token, "X-Keldock-Agent-Session": access.NewID("run"), "X-Keldock-Agent-Version": "m2-test"}
	fixture.call(t, anonymous, http.MethodPost, "/v1/agent/heartbeat", headers, nil, nil, http.StatusOK)
	fixture.call(t, anonymous, http.MethodGet, "/v1/users", headers, nil, nil, http.StatusUnauthorized)
	fixture.call(t, fixture.client, http.MethodPatch, "/v1/mcp/settings", nil, map[string]any{"enabled": true, "version": 1}, nil, http.StatusOK)
	fixture.call(t, fixture.client, http.MethodPost, "/v1/clusters/keldock-m1/revoke", nil, nil, nil, http.StatusOK)
	fixture.call(t, anonymous, http.MethodPost, "/v1/agent/heartbeat", headers, nil, nil, http.StatusForbidden)
	fixture.call(t, anonymous, http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil, nil, nil, http.StatusOK)
}

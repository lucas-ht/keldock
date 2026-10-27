package controlplane

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lucas-ht/keldock/internal/admission"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// API exposes one experimental installation's shared authenticated product routes.
// Static operator/agent credentials are an M1 development profile, not hosted authentication.
type API struct {
	Store            *postgres.Store
	GitHub           *admission.GitHub
	Organization     string
	Cluster          string
	Projects         map[string]v1.Project
	RepositoryOwners map[int64]int64
	OperatorToken    string
	AgentToken       string
	WebhookSecret    []byte
	Domain           string
}

// Handler connects CI, status, webhook and outbound-agent routes to the same stored authority.
func (api *API) Handler() http.Handler {
	if api.Store == nil || api.GitHub == nil || !v1.ValidName(api.Organization) || !v1.ValidName(api.Cluster) || len(api.Projects) == 0 || len(api.OperatorToken) < 32 || len(api.AgentToken) < 32 || api.OperatorToken == api.AgentToken || len(api.WebhookSecret) < 32 || api.Domain == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			problem(w, http.StatusServiceUnavailable, "installation_not_configured", "Installation configuration is incomplete")
		})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/releases", api.submit)
	mux.HandleFunc("GET /v1/projects/{project}/operations/{operation}", api.status)
	mux.HandleFunc("POST /v1/projects/{project}/operations/{operation}/cancel", api.cancel)
	mux.HandleFunc("POST /v1/github/webhooks", api.webhook)
	mux.HandleFunc("GET /v1/agent/desired", api.desired)
	mux.HandleFunc("GET /v1/agent/environments/{environment}", api.currentDesired)
	mux.HandleFunc("POST /v1/agent/observations", api.observe)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

func bearer(request *http.Request) string {
	value := request.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(value, "Bearer ")
}

func matches(token, expected string) bool {
	return len(expected) >= 32 && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func (api *API) scope(request *http.Request) (v1.Scope, string, error) {
	project, exists := api.Projects[request.PathValue("project")]
	if !exists || project.OrganizationID != api.Organization {
		return v1.Scope{}, "", postgres.ErrDenied
	}
	if matches(bearer(request), api.OperatorToken) {
		return project.Scope, "operator", nil
	}
	principal, err := api.GitHub.Authenticate(request.Context(), bearer(request))
	if err != nil || principal.Scope != project.Scope {
		return v1.Scope{}, "", postgres.ErrDenied
	}
	return principal.Scope, principal.Actor, nil
}

func decode(w http.ResponseWriter, request *http.Request, target any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, 1024*1024))
	if err == nil {
		err = v1.DecodeJSON(body, target)
	}
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_request", "Invalid or oversized request contract")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Debug("response interrupted", "error", err)
	}
}

func problem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, postgres.ErrDenied), errors.Is(err, admission.ErrDenied):
		problem(w, http.StatusForbidden, "denied", "Request outside authorized scope")
	case errors.Is(err, postgres.ErrNotFound):
		problem(w, http.StatusNotFound, "not_found", "Target not found")
	case errors.Is(err, postgres.ErrConflict), errors.Is(err, postgres.ErrStale):
		problem(w, http.StatusConflict, "conflict", "Conflicting or stale desired revision")
	default:
		slog.Error("API operation failed", "error", err)
		problem(w, http.StatusInternalServerError, "unavailable", "Operation unavailable; retry using the same request identity")
	}
}

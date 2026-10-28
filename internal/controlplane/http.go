package controlplane

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/admission"
	cloudstore "github.com/lucas-ht/keldock/internal/cloud/store"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane/authentication"
	githubhttp "github.com/lucas-ht/keldock/internal/controlplane/github"
	"github.com/lucas-ht/keldock/internal/githubapp"
	"github.com/lucas-ht/keldock/internal/mcpserver"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

// API exposes one experimental installation's shared authenticated product routes.
type API struct {
	Store            *postgres.Store
	GitHub           *admission.GitHub
	GitHubApp        *githubapp.Service
	Organization     string
	Cluster          string
	Projects         map[string]v1.Project
	RepositoryOwners map[int64]int64
	Security         *authentication.Service
	WebhookSecret    []byte
	Development      bool
	Notice           string
	Domain           string
	Cloud            *cloudstore.Store
}

// Handler connects CI, status, webhook and outbound-agent routes to the same stored authority.
func (api *API) Handler() http.Handler {
	if api.Store == nil || api.Security == nil || !v1.ValidName(api.Organization) || !v1.ValidName(api.Cluster) || api.Domain == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			problem(w, http.StatusServiceUnavailable, "installation_not_configured", "Installation configuration is incomplete")
		})
	}
	mux := http.NewServeMux()
	api.Security.Register(mux)
	api.registerCloud(mux)
	if api.GitHubApp != nil {
		(githubhttp.Service{Integration: api.GitHubApp, Security: api.Security}).Register(mux)
	}
	mux.Handle("/mcp", mcpserver.Handler(api.Security.MCP, api.inspector(), api.operatorService()))
	mux.HandleFunc("GET /v1/installation", api.installation)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/releases", api.submit)
	mux.HandleFunc("GET /v1/projects", api.projects)
	mux.HandleFunc("GET /v1/projects/{project}/releases", api.releases)
	mux.HandleFunc("GET /v1/projects/{project}/environments", api.environments)
	mux.HandleFunc("POST /v1/projects/{project}/environments/{environment}/deployments", api.selectRelease)
	mux.HandleFunc("GET /v1/projects/{project}/environments/{environment}", api.environment)
	mux.HandleFunc("GET /v1/projects/{project}/environments/{environment}/diagnostics", api.diagnostics)
	mux.HandleFunc("GET /v1/projects/{project}/environments/{environment}/timeline", api.timeline)
	mux.HandleFunc("POST /v1/projects/{project}/operations/{operation}/retry", api.retry)
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
	return authentication.Bearer(request)
}

func (api *API) scope(request *http.Request) (v1.Scope, string, error) {
	project, exists := api.Projects[request.PathValue("project")]
	if !exists || project.OrganizationID != api.Organization {
		return v1.Scope{}, "", postgres.ErrDenied
	}
	reader, err := api.Security.Reader(request)
	if err == nil {
		if !reader.CanRead(project.ProjectID) {
			return v1.Scope{}, "", access.ErrDenied
		}
		return project.Scope, "user:" + reader.User.ID, nil
	}
	if !authentication.IsIdentityError(err) {
		return v1.Scope{}, "", err
	}
	if api.GitHub == nil {
		return v1.Scope{}, "", access.ErrDenied
	}
	principal, err := api.GitHub.Authenticate(request.Context(), bearer(request))
	if err != nil || principal.Scope != project.Scope {
		return v1.Scope{}, "", postgres.ErrDenied
	}
	if api.GitHubApp != nil {
		if _, err := api.GitHubApp.AuthorizeRepository(request.Context(), principal.Source.RepositoryID, principal.Source.OwnerID, principal.Source.Repository); err != nil {
			return v1.Scope{}, "", err
		}
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
	case errors.Is(err, postgres.ErrLimited):
		problem(w, http.StatusTooManyRequests, "rate_limited", "Accepted operation limit reached; retry later with the same request identity")
	case errors.Is(err, access.ErrInvalid):
		problem(w, http.StatusBadRequest, "invalid_request", "Check target identifiers, generation and request identity")
	case errors.Is(err, postgres.ErrQuota):
		problem(w, http.StatusConflict, "quota_exceeded", "Preview capacity is reserved; wait for cleanup or ask an operator to review the project limit")
	case errors.Is(err, postgres.ErrDenied), errors.Is(err, admission.ErrDenied), errors.Is(err, access.ErrDenied):
		problem(w, http.StatusForbidden, "denied", "Request outside authorized scope")
	case errors.Is(err, postgres.ErrNotFound), errors.Is(err, access.ErrNotFound):
		problem(w, http.StatusNotFound, "not_found", "Target not found")
	case errors.Is(err, postgres.ErrConflict), errors.Is(err, postgres.ErrStale), errors.Is(err, access.ErrConflict), errors.Is(err, access.ErrSessionBusy):
		problem(w, http.StatusConflict, "conflict", "Conflicting or stale desired revision")
	default:
		slog.Error("API operation failed", "error", err)
		problem(w, http.StatusServiceUnavailable, "unavailable", "Operation unavailable; retry using the same request identity")
	}
}

package controlplane

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/inspection"
)

func (api *API) inspector() *inspection.Service {
	return &inspection.Service{Store: api.Store, Organization: api.Organization, Projects: api.Projects, PublicURL: api.Security.MCP.Issuer}
}

func (api *API) reader(w http.ResponseWriter, request *http.Request) (access.Principal, bool) {
	principal, err := api.Security.Reader(request)
	if err != nil {
		failure(w, err)
		return principal, false
	}
	return principal, true
}

func (api *API) projects(w http.ResponseWriter, request *http.Request) {
	principal, ok := api.reader(w, request)
	if !ok {
		return
	}
	result, err := api.inspector().ListProjects(request.Context(), principal)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) environments(w http.ResponseWriter, request *http.Request) {
	principal, ok := api.reader(w, request)
	if !ok {
		return
	}
	result, err := api.inspector().ListEnvironments(request.Context(), principal, request.PathValue("project"), request.URL.Query().Get("after"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) environment(w http.ResponseWriter, request *http.Request) {
	principal, ok := api.reader(w, request)
	if !ok {
		return
	}
	result, err := api.inspector().GetEnvironment(request.Context(), principal, request.PathValue("project"), request.PathValue("environment"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) diagnostics(w http.ResponseWriter, request *http.Request) {
	principal, ok := api.reader(w, request)
	if !ok {
		return
	}
	result, err := api.inspector().GetDiagnostics(request.Context(), principal, request.PathValue("project"), request.PathValue("environment"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) timeline(w http.ResponseWriter, request *http.Request) {
	principal, ok := api.reader(w, request)
	if !ok {
		return
	}
	result, err := api.inspector().Timeline(request.Context(), principal, request.PathValue("project"), request.PathValue("environment"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (api *API) installation(w http.ResponseWriter, request *http.Request) {
	if _, ok := api.reader(w, request); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizationId": api.Organization, "clusterId": api.Cluster, "development": api.Development, "notice": api.Notice})
}

func (api *API) releases(w http.ResponseWriter, r *http.Request) {
	principal, ok := api.reader(w, r)
	if !ok {
		return
	}
	result, err := api.inspector().ListReleases(r.Context(), principal, r.PathValue("project"), r.URL.Query().Get("after"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

package controlplane

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/inspection"
)

func (api *API) inspector() *inspection.Service {
	return &inspection.Service{Store: api.Store, Organization: api.Organization, Projects: api.Projects}
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

func (api *API) retry(w http.ResponseWriter, request *http.Request) {
	principal, err := api.Security.Mutation(request)
	if err != nil {
		failure(w, err)
		return
	}
	project, exists := api.Projects[request.PathValue("project")]
	if !exists || !principal.CanOperate(project.ProjectID) {
		failure(w, access.ErrDenied)
		return
	}
	var input struct {
		Generation int64 `json:"generation"`
	}
	if !decode(w, request, &input) {
		return
	}
	result, err := api.Store.Retry(request.Context(), project.Scope, "user:"+principal.User.ID, request.PathValue("operation"), input.Generation, request.Header.Get("Idempotency-Key"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (api *API) installation(w http.ResponseWriter, request *http.Request) {
	if _, ok := api.reader(w, request); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizationId": api.Organization, "clusterId": api.Cluster, "development": api.Development, "notice": api.Notice})
}

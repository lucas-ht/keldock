package controlplane

import (
	"net/http"
	"strings"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (api *API) submit(w http.ResponseWriter, request *http.Request) {
	if api.GitHub == nil {
		problem(w, http.StatusServiceUnavailable, "ci_not_configured", "Configure a trusted CI issuer before submitting releases")
		return
	}
	var input v1.SubmitRelease
	if !decode(w, request, &input) {
		return
	}
	if err := input.Validate(); err != nil {
		problem(w, http.StatusBadRequest, "invalid_release", "Release does not satisfy the supported immutable application contract")
		return
	}
	principal, err := api.GitHub.Authenticate(request.Context(), bearer(request))
	if err != nil {
		failure(w, err)
		return
	}
	if err := api.GitHub.VerifyRelease(principal, input); err != nil {
		failure(w, err)
		return
	}
	if api.GitHubApp != nil {
		if err := api.GitHubApp.AuthorizeRelease(request.Context(), input.Source); err != nil {
			failure(w, err)
			return
		}
	}
	project, ok := api.Projects[principal.ProjectID]
	if !ok || project.Scope != principal.Scope {
		failure(w, postgres.ErrDenied)
		return
	}
	environment := v1.EnvironmentID(principal.Scope, input.Definition.Name, input.Source.PullRequest)
	hostname := "preview-" + strings.TrimPrefix(environment, "env-")[:12] + "." + api.Domain
	if input.Source.PullRequest == 0 {
		hostname = "staging-" + strings.TrimPrefix(environment, "env-")[:12] + "." + api.Domain
	}
	receipt, err := api.Store.Admit(request.Context(), principal.Scope, principal.Actor, request.Header.Get("Idempotency-Key"), hostname, input)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Location", receipt.StatusURL)
	writeJSON(w, http.StatusAccepted, receipt)
}

func (api *API) status(w http.ResponseWriter, request *http.Request) {
	scope, _, err := api.scope(request)
	if err != nil {
		failure(w, err)
		return
	}
	reader := access.Principal{Kind: "ci", User: access.User{OrganizationID: scope.OrganizationID, Status: "active", Role: "viewer", Projects: []string{scope.ProjectID}}}
	result, err := api.inspector().GetOperation(request.Context(), reader, scope.ProjectID, request.PathValue("operation"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) cancel(w http.ResponseWriter, request *http.Request) {
	principal, err := api.Security.Mutation(request)
	project, exists := api.Projects[request.PathValue("project")]
	if err != nil {
		failure(w, err)
		return
	}
	if !exists || !principal.CanOperate(project.ProjectID) {
		failure(w, postgres.ErrDenied)
		return
	}
	if err := api.Store.Cancel(request.Context(), project.Scope, request.PathValue("operation"), "user:"+principal.User.ID); err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "canceled", "detail": "Completed external effects are retained; this is not a rollback"})
}

func (api *API) desired(w http.ResponseWriter, request *http.Request) {
	if _, err := api.Security.Agent(request); err != nil {
		failure(w, err)
		return
	}
	items, next, err := api.Store.DesiredPage(request.Context(), api.Organization, api.Cluster, request.URL.Query().Get("after"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"apiVersion": v1.APIVersion, "items": items, "nextCursor": next})
}

func (api *API) currentDesired(w http.ResponseWriter, request *http.Request) {
	if _, err := api.Security.Agent(request); err != nil {
		failure(w, err)
		return
	}
	desired, err := api.Store.CurrentDesired(request.Context(), api.Organization, api.Cluster, request.PathValue("environment"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, desired)
}

func (api *API) observe(w http.ResponseWriter, request *http.Request) {
	if _, err := api.Security.Agent(request); err != nil {
		failure(w, err)
		return
	}
	var report v1.Observation
	if !decode(w, request, &report) {
		return
	}
	if err := api.Store.Observe(request.Context(), api.Organization, api.Cluster, report); err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "recorded"})
}

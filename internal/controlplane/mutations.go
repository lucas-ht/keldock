package controlplane

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/operations"
)

func (api *API) operatorService() *operations.Service {
	service := &operations.Service{Store: api.Store, Users: api.Security.Users, MCP: api.Security.MCP, Projects: api.Projects, Organization: api.Organization}
	if api.GitHubApp != nil {
		service.GitHub = api.GitHubApp
	}
	return service
}

func (api *API) operator(w http.ResponseWriter, r *http.Request) (access.Principal, bool) {
	principal, err := api.Security.OperationMutation(r)
	if err != nil {
		failure(w, err)
		return principal, false
	}
	if _, ok := api.Projects[r.PathValue("project")]; !ok || !principal.CanOperate(r.PathValue("project")) {
		failure(w, access.ErrDenied)
		return principal, false
	}
	return principal, true
}

func (api *API) selectRelease(w http.ResponseWriter, r *http.Request) {
	principal, ok := api.operator(w, r)
	if !ok {
		return
	}
	var input struct {
		ReleaseID  string `json:"releaseId"`
		Generation int64  `json:"generation"`
	}
	if !decode(w, r, &input) {
		return
	}
	receipt, err := api.operatorService().Select(r.Context(), principal, r.PathValue("project"), v1.ReleaseSelection{ReleaseID: input.ReleaseID, EnvironmentID: r.PathValue("environment"), Generation: input.Generation, RequestKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

func (api *API) retry(w http.ResponseWriter, r *http.Request) {
	principal, ok := api.operator(w, r)
	if !ok {
		return
	}
	var input struct {
		Generation int64 `json:"generation"`
	}
	if !decode(w, r, &input) {
		return
	}
	receipt, err := api.operatorService().Retry(r.Context(), principal, r.PathValue("project"), v1.OperationMutation{OperationID: r.PathValue("operation"), Generation: input.Generation, RequestKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

func (api *API) cancel(w http.ResponseWriter, r *http.Request) {
	principal, ok := api.operator(w, r)
	if !ok {
		return
	}
	var input struct {
		Generation int64 `json:"generation"`
	}
	key := r.Header.Get("Idempotency-Key")
	if r.ContentLength == 0 && principal.Kind == "session" {
		// Legacy session requests already bind an immutable operation ID. Preserve that
		// contract while modern UI/MCP callers supply an explicit generation and key.
		operation, err := api.Store.Operation(r.Context(), api.Projects[r.PathValue("project")].Scope, r.PathValue("operation"))
		if err != nil {
			failure(w, err)
			return
		}
		input.Generation = operation.Generation
		if key == "" {
			key = "cancel:" + operation.ID
		}
	} else if !decode(w, r, &input) {
		return
	}
	receipt, err := api.operatorService().Cancel(r.Context(), principal, r.PathValue("project"), v1.OperationMutation{OperationID: r.PathValue("operation"), Generation: input.Generation, RequestKey: key})
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, struct {
		v1.Receipt
		State  string `json:"state"`
		Detail string `json:"detail"`
	}{receipt, "canceled", "Completed external effects are retained; this is not a rollback"})
}

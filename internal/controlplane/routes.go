package controlplane

import (
	"net/http"
	"strings"
	"time"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (api *API) submit(w http.ResponseWriter, request *http.Request) {
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
	operation, err := api.Store.Operation(request.Context(), scope, request.PathValue("operation"))
	if err != nil {
		failure(w, err)
		return
	}
	observation, err := api.Store.LatestObservation(request.Context(), scope, operation.ID)
	if err != nil {
		failure(w, err)
		return
	}
	stale := true
	var age float64
	if observation != nil {
		age = max(0, time.Since(observation.ObservedAt).Seconds())
		stale = age > 30
	}
	writeJSON(w, http.StatusOK, map[string]any{"apiVersion": v1.APIVersion, "operation": operation, "observation": observation, "observationAgeSeconds": age, "stale": stale})
}

func (api *API) cancel(w http.ResponseWriter, request *http.Request) {
	if !matches(bearer(request), api.OperatorToken) {
		failure(w, postgres.ErrDenied)
		return
	}
	scope, actor, err := api.scope(request)
	if err != nil {
		failure(w, err)
		return
	}
	if err := api.Store.Cancel(request.Context(), scope, request.PathValue("operation"), actor); err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "canceled", "detail": "Completed external effects are retained; this is not a rollback"})
}

func (api *API) desired(w http.ResponseWriter, request *http.Request) {
	if !matches(bearer(request), api.AgentToken) {
		failure(w, postgres.ErrDenied)
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
	if !matches(bearer(request), api.AgentToken) {
		failure(w, postgres.ErrDenied)
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
	if !matches(bearer(request), api.AgentToken) {
		failure(w, postgres.ErrDenied)
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

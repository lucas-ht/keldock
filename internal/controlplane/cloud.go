package controlplane

import (
	"errors"
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/cloud"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (api *API) registerCloud(mux *http.ServeMux) {
	if api.Cloud == nil {
		return
	}
	mux.HandleFunc("GET /v1/projects/{project}/cloud", api.cloudInventory)
	mux.HandleFunc("GET /v1/projects/{project}/cloud/intents/{intent}", api.cloudIntent)
	mux.HandleFunc("POST /v1/projects/{project}/cloud", api.cloudMutate)
	mux.HandleFunc("GET /v1/cloud/agent/desired", api.cloudDesired)
	mux.HandleFunc("GET /v1/cloud/agent/intents/{intent}", api.cloudCurrent)
	mux.HandleFunc("POST /v1/cloud/agent/observations", api.cloudObserve)
}

func (api *API) cloudInventory(w http.ResponseWriter, r *http.Request) {
	principal, err := api.Security.Reader(r)
	if err != nil {
		failure(w, err)
		return
	}
	items, err := api.Cloud.List(r.Context(), principal, r.PathValue("project"))
	if err != nil {
		failure(w, err)
		return
	}
	catalog, err := api.Cloud.Catalog(principal, r.PathValue("project"))
	if err != nil {
		failure(w, err)
		return
	}
	cloudAgent, err := api.Security.Cloud.Information(r.Context())
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "configuration": catalog, "agent": cloudAgent})
}

func (api *API) cloudIntent(w http.ResponseWriter, r *http.Request) {
	principal, err := api.Security.Reader(r)
	if err != nil {
		failure(w, err)
		return
	}
	items, err := api.Cloud.List(r.Context(), principal, r.PathValue("project"))
	if err != nil {
		failure(w, err)
		return
	}
	for _, item := range items {
		if item.ID == r.PathValue("intent") {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	failure(w, postgres.ErrNotFound)
}

func (api *API) cloudMutate(w http.ResponseWriter, r *http.Request) {
	principal, err := api.Security.Mutation(r)
	if err != nil {
		failure(w, err)
		return
	}
	if principal.Kind != "session" {
		failure(w, access.ErrDenied)
		return
	}
	var command cloud.Command
	if !decode(w, r, &command) {
		return
	}
	receipt, err := api.Cloud.Mutate(r.Context(), principal, r.PathValue("project"), r.Header.Get("Idempotency-Key"), command)
	if err != nil {
		if errors.Is(err, postgres.ErrQuota) {
			problem(w, http.StatusConflict, "quota_exceeded", "Approved cloud resource capacity is reserved; disconnect unused resources or ask the installation operator to review the limit")
			return
		}
		failure(w, err)
		return
	}
	w.Header().Set("Location", receipt.StatusURL)
	writeJSON(w, http.StatusAccepted, receipt)
}

func (api *API) cloudDesired(w http.ResponseWriter, r *http.Request) {
	identity, err := api.Security.CloudAgent(r)
	if err != nil {
		failure(w, err)
		return
	}
	items, err := api.Cloud.Desired(r.Context(), identity.OrganizationID, identity.ClusterID)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (api *API) cloudCurrent(w http.ResponseWriter, r *http.Request) {
	identity, err := api.Security.CloudAgent(r)
	if err != nil {
		failure(w, err)
		return
	}
	items, err := api.Cloud.Desired(r.Context(), identity.OrganizationID, identity.ClusterID)
	if err != nil {
		failure(w, err)
		return
	}
	for _, item := range items {
		if item.ID == r.PathValue("intent") {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	failure(w, postgres.ErrNotFound)
}

func (api *API) cloudObserve(w http.ResponseWriter, r *http.Request) {
	identity, err := api.Security.CloudAgent(r)
	if err != nil {
		failure(w, err)
		return
	}
	var report cloud.Evidence
	if !decode(w, r, &report) {
		return
	}
	if err := api.Cloud.Observe(r.Context(), identity.OrganizationID, identity.ClusterID, report); err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "recorded"})
}

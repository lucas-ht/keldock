package controlplane

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/database"
	databasestore "github.com/lucas-ht/keldock/internal/database/store"
	"github.com/lucas-ht/keldock/internal/storage/postgres"
)

func (api *API) registerDatabases(mux *http.ServeMux) {
	if api.Databases == nil {
		return
	}
	mux.HandleFunc("GET /v1/projects/{project}/databases", api.databases)
	mux.HandleFunc("POST /v1/projects/{project}/databases", api.databaseMutate)
	mux.HandleFunc("GET /v1/database/agent/desired", api.databaseDesired)
	mux.HandleFunc("GET /v1/database/agent/allocations/{allocation}", api.databaseCurrent)
	mux.HandleFunc("POST /v1/database/agent/observations", api.databaseObserve)
}

func (api *API) databases(w http.ResponseWriter, r *http.Request) {
	p, ok := api.reader(w, r)
	if !ok {
		return
	}
	items, err := api.Databases.List(r.Context(), p, r.PathValue("project"))
	if err != nil {
		failure(w, err)
		return
	}
	identity, err := api.Security.Database.Information(r.Context())
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "configuration": api.Databases.Policies[r.PathValue("project")], "agent": identity})
}

func (api *API) databaseMutate(w http.ResponseWriter, r *http.Request) {
	p, err := api.Security.Mutation(r)
	if err != nil {
		failure(w, err)
		return
	}
	var command databasestore.Command
	if !decode(w, r, &command) {
		return
	}
	item, err := api.Databases.Mutate(r.Context(), p, r.PathValue("project"), r.Header.Get("Idempotency-Key"), command)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (api *API) databaseDesired(w http.ResponseWriter, r *http.Request) {
	identity, err := api.Security.DatabaseAgent(r)
	if err != nil {
		failure(w, err)
		return
	}
	items, err := api.Databases.Desired(r.Context(), identity.OrganizationID, identity.ClusterID)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (api *API) databaseCurrent(w http.ResponseWriter, r *http.Request) {
	identity, err := api.Security.DatabaseAgent(r)
	if err != nil {
		failure(w, err)
		return
	}
	items, err := api.Databases.Desired(r.Context(), identity.OrganizationID, identity.ClusterID)
	if err != nil {
		failure(w, err)
		return
	}
	for _, item := range items {
		if item.ID == r.PathValue("allocation") && item.ProjectID == r.URL.Query().Get("project") {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	failure(w, postgres.ErrNotFound)
}

func (api *API) databaseObserve(w http.ResponseWriter, r *http.Request) {
	identity, err := api.Security.DatabaseAgent(r)
	if err != nil {
		failure(w, err)
		return
	}
	var evidence database.Evidence
	if !decode(w, r, &evidence) {
		return
	}
	if err := api.Databases.Observe(r.Context(), identity.OrganizationID, identity.ClusterID, evidence); err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "recorded"})
}

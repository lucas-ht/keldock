package authentication

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
)

func (service *Service) clusters(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, false)
	if !ok {
		return
	}
	allowed := principal.IsAdmin()
	for _, project := range service.config.Projects {
		allowed = allowed || principal.CanRead(project)
	}
	if !allowed {
		write(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	cluster, err := service.Clusters.Information(request.Context())
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"items": []any{cluster}})
}

func (service *Service) enrollment(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	if request.PathValue("cluster") != service.config.ClusterID {
		fail(w, access.ErrNotFound)
		return
	}
	grant, err := service.Clusters.CreateEnrollment(request.Context(), principal)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, map[string]any{"apiUrl": service.config.PublicURL, "enrollment": grant})
}

func (service *Service) revokeCluster(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	if request.PathValue("cluster") != service.config.ClusterID {
		fail(w, access.ErrNotFound)
		return
	}
	if err := service.Clusters.Revoke(request.Context(), principal); err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"state": "revoked", "detail": "Existing workloads retain their last approved state"})
}

func (service *Service) enroll(w http.ResponseWriter, request *http.Request) {
	var input struct {
		ClusterID string `json:"clusterId"`
		Token     string `json:"token"`
	}
	if !decode(w, request, &input) {
		return
	}
	identity, err := service.Clusters.Exchange(request.Context(), input.ClusterID, input.Token)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, identity)
}

func (service *Service) rotate(w http.ResponseWriter, request *http.Request) {
	var input struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if !decode(w, request, &input) {
		return
	}
	identity, err := service.Clusters.Rotate(request.Context(), input.ID, Bearer(request), input.Token, request.Header.Get("X-Keldock-Agent-Session"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, identity)
}

func (service *Service) agentIdentity(w http.ResponseWriter, request *http.Request) {
	identity, err := service.Agent(request)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, identity)
}

func (service *Service) heartbeat(w http.ResponseWriter, request *http.Request) {
	if _, err := service.Agent(request); err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"state": "connected"})
}

func (service *Service) releaseAgent(w http.ResponseWriter, request *http.Request) {
	if _, err := service.Agent(request); err != nil {
		fail(w, err)
		return
	}
	if err := service.Clusters.Release(request.Context(), Bearer(request), request.Header.Get("X-Keldock-Agent-Session")); err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"state": "released"})
}

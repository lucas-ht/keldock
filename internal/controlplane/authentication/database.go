package authentication

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
)

func (service *Service) registerDatabase(mux *http.ServeMux) {
	if service.Database == nil {
		return
	}
	database := *service
	database.Clusters = service.Database
	mux.HandleFunc("GET /v1/database/clusters", database.clusters)
	mux.HandleFunc("POST /v1/database/clusters/{cluster}/enrollment", database.enrollment)
	mux.HandleFunc("POST /v1/database/clusters/{cluster}/revoke", database.revokeCluster)
	mux.HandleFunc("POST /v1/database/agent/enroll", database.enroll)
	mux.HandleFunc("POST /v1/database/agent/rotate", database.rotate)
	mux.HandleFunc("GET /v1/database/agent/identity", database.agentIdentity)
	mux.HandleFunc("POST /v1/database/agent/heartbeat", database.heartbeat)
	mux.HandleFunc("POST /v1/database/agent/session/release", database.releaseAgent)
}

// DatabaseAgent accepts only the independently enrolled PostgreSQL provisioner.
func (service *Service) DatabaseAgent(r *http.Request) (clusteridentity.Identity, error) {
	if service.Database == nil {
		return clusteridentity.Identity{}, access.ErrDenied
	}
	return service.Database.Authenticate(r.Context(), Bearer(r), r.Header.Get("X-Keldock-Agent-Session"), r.Header.Get("X-Keldock-Agent-Version"))
}

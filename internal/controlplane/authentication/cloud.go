package authentication

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
)

func (service *Service) registerCloud(mux *http.ServeMux) {
	if service.Cloud == nil {
		return
	}
	cloud := *service
	cloud.Clusters = service.Cloud
	mux.HandleFunc("GET /v1/cloud/clusters", cloud.clusters)
	mux.HandleFunc("POST /v1/cloud/clusters/{cluster}/enrollment", cloud.enrollment)
	mux.HandleFunc("POST /v1/cloud/clusters/{cluster}/revoke", cloud.revokeCluster)
	mux.HandleFunc("POST /v1/cloud/agent/enroll", cloud.enroll)
	mux.HandleFunc("POST /v1/cloud/agent/rotate", cloud.rotate)
	mux.HandleFunc("GET /v1/cloud/agent/identity", cloud.agentIdentity)
	mux.HandleFunc("POST /v1/cloud/agent/heartbeat", cloud.heartbeat)
	mux.HandleFunc("POST /v1/cloud/agent/session/release", cloud.releaseAgent)
}

// CloudAgent checks only provisioning credentials; registrar credentials cannot cross this boundary.
func (service *Service) CloudAgent(request *http.Request) (clusteridentity.Identity, error) {
	if service.Cloud == nil {
		return clusteridentity.Identity{}, access.ErrDenied
	}
	return service.Cloud.Authenticate(request.Context(), Bearer(request), request.Header.Get("X-Keldock-Agent-Session"), request.Header.Get("X-Keldock-Agent-Version"))
}

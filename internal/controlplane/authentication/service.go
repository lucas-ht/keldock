// Package authentication exposes the installation's shared human, agent and MCP authority.
package authentication

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
)

// Config contains installation authority, never values accepted from application input.
type Config struct {
	PublicURL   string
	Development bool
	ClusterID   string
	Projects    []string
}

// Service shares one authorization boundary across UI, CI reads, cluster RPC and MCP.
type Service struct {
	Users    *access.Repository
	Clusters *clusteridentity.Repository
	Cloud    *clusteridentity.Repository
	Database *clusteridentity.Repository
	MCP      *mcpidentity.Repository
	config   Config
	cookie   string
	secure   bool
	hashing  chan struct{}
}

// New requires HTTPS outside the explicit loopback development profile.
func New(users *access.Repository, clusters *clusteridentity.Repository, mcp *mcpidentity.Repository, config Config) (*Service, error) {
	parsed, err := url.Parse(config.PublicURL)
	if users == nil || clusters == nil || mcp == nil || err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, access.ErrInvalid
	}
	local := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if parsed.Scheme != "https" && (!config.Development || parsed.Scheme != "http" || !local) {
		return nil, access.ErrInvalid
	}
	config.PublicURL = strings.TrimSuffix(config.PublicURL, "/")
	cookie := "__Host-keldock_session"
	if parsed.Scheme == "http" {
		cookie = "keldock_session"
	}
	return &Service{Users: users, Clusters: clusters, MCP: mcp, config: config, cookie: cookie, secure: parsed.Scheme == "https", hashing: make(chan struct{}, 4)}, nil
}

// Register installs concrete authenticated management and OAuth endpoints.
func (service *Service) Register(mux *http.ServeMux) {
	service.registerCloud(mux)
	service.registerDatabase(mux)
	mux.HandleFunc("POST /v1/auth/login", service.login)
	mux.HandleFunc("POST /v1/auth/logout", service.logout)
	mux.HandleFunc("GET /v1/auth/me", service.me)
	mux.HandleFunc("POST /v1/auth/invitations/info", service.invitationInfo)
	mux.HandleFunc("POST /v1/auth/activate", service.activate)
	mux.HandleFunc("POST /v1/auth/password", service.password)
	mux.HandleFunc("GET /v1/users", service.users)
	mux.HandleFunc("GET /v1/users/{user}", service.user)
	mux.HandleFunc("POST /v1/users", service.invite)
	mux.HandleFunc("PATCH /v1/users/{user}", service.updateUser)
	mux.HandleFunc("POST /v1/users/{user}/invitation", service.reissueInvitation)
	mux.HandleFunc("GET /v1/clusters", service.clusters)
	mux.HandleFunc("POST /v1/clusters/{cluster}/enrollment", service.enrollment)
	mux.HandleFunc("POST /v1/clusters/{cluster}/revoke", service.revokeCluster)
	mux.HandleFunc("POST /v1/agent/enroll", service.enroll)
	mux.HandleFunc("POST /v1/agent/rotate", service.rotate)
	mux.HandleFunc("GET /v1/agent/identity", service.agentIdentity)
	mux.HandleFunc("POST /v1/agent/heartbeat", service.heartbeat)
	mux.HandleFunc("POST /v1/agent/session/release", service.releaseAgent)
	mux.HandleFunc("GET /v1/mcp/settings", service.mcpSettings)
	mux.HandleFunc("PATCH /v1/mcp/settings", service.setMCPSettings)
	mux.HandleFunc("GET /v1/mcp/clients", service.mcpClients)
	mux.HandleFunc("POST /v1/mcp/clients", service.registerMCPClient)
	mux.HandleFunc("POST /v1/mcp/clients/{client}/disable", service.disableMCPClient)
	mux.HandleFunc("GET /v1/mcp/connections", service.connections)
	mux.HandleFunc("POST /v1/mcp/connections/{connection}/revoke", service.revokeConnection)
	mux.HandleFunc("GET /v1/mcp/authorizations/{request}", service.authorization)
	mux.HandleFunc("POST /v1/mcp/authorizations/{request}/consent", service.consent)
	service.registerOAuth(mux)
}

// CookieName lets trusted clients/tests identify the deployment-specific session cookie.
func (service *Service) CookieName() string { return service.cookie }

func (service *Service) sessionToken(request *http.Request) string {
	cookie, err := request.Cookie(service.cookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// Human authenticates only a browser session, never an agent or MCP bearer.
func (service *Service) Human(request *http.Request) (access.Principal, error) {
	principal, _, err := service.Users.Session(request.Context(), service.sessionToken(request))
	return principal, err
}

// Reader accepts a human session or the narrower OAuth read credential.
func (service *Service) Reader(request *http.Request) (access.Principal, error) {
	if service.sessionToken(request) != "" {
		return service.Human(request)
	}
	principal, _, err := service.MCP.Authenticate(request.Context(), Bearer(request), service.MCP.Resource)
	return principal, err
}

// Agent checks durable cluster revocation and per-process lease ownership on every RPC.
func (service *Service) Agent(request *http.Request) (clusteridentity.Identity, error) {
	return service.Clusters.Authenticate(request.Context(), Bearer(request), request.Header.Get("X-Keldock-Agent-Session"), request.Header.Get("X-Keldock-Agent-Version"))
}

// Bearer parses only the supported authorization scheme without reflecting its contents.
func Bearer(request *http.Request) string {
	value := request.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(value, "Bearer ")
}

// OperationMutation accepts a CSRF-protected session or a separately scoped MCP bearer.
// Cookie-bearing requests always retain browser CSRF checks, even with a bearer present.
func (service *Service) OperationMutation(request *http.Request) (access.Principal, error) {
	if service.sessionToken(request) != "" {
		return service.Mutation(request)
	}
	principal, _, err := service.MCP.Authenticate(request.Context(), Bearer(request), service.MCP.Resource)
	return principal, err
}

// Mutation rechecks the trusted origin, live session and session-bound anti-forgery proof.
func (service *Service) Mutation(request *http.Request) (access.Principal, error) {
	if request.Header.Get("Origin") != service.config.PublicURL || !access.ValidCSRF(service.sessionToken(request), request.Header.Get("X-Keldock-CSRF")) {
		return access.Principal{}, access.ErrDenied
	}
	return service.Human(request)
}

func (service *Service) human(w http.ResponseWriter, request *http.Request, mutating bool) (access.Principal, bool) {
	principal, err := service.Human(request)
	if IsIdentityError(err) {
		problem(w, http.StatusUnauthorized, "not_authenticated", "Sign in to continue")
		return principal, false
	}
	if err != nil {
		fail(w, err)
		return principal, false
	}
	if mutating && (request.Header.Get("Origin") != service.config.PublicURL || !access.ValidCSRF(service.sessionToken(request), request.Header.Get("X-Keldock-CSRF"))) {
		fail(w, access.ErrDenied)
		return principal, false
	}
	return principal, true
}

func (service *Service) origin(w http.ResponseWriter, request *http.Request) bool {
	if request.Header.Get("Origin") != service.config.PublicURL {
		problem(w, http.StatusForbidden, "origin_denied", "Request origin is not allowed")
		return false
	}
	return true
}

func (service *Service) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: service.cookie, Value: token, Path: "/", HttpOnly: true, Secure: service.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(access.SessionLifetime.Seconds()), Expires: time.Now().Add(access.SessionLifetime)})
}

func (service *Service) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: service.cookie, Path: "/", HttpOnly: true, Secure: service.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (service *Service) acquireHash(ctx context.Context) (func(), error) {
	select {
	case service.hashing <- struct{}{}:
		return func() { <-service.hashing }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, access.ErrLimited
	}
}

// IsIdentityError distinguishes a rejected credential from a dependency outage.
func IsIdentityError(err error) bool {
	return errors.Is(err, access.ErrDenied) || errors.Is(err, access.ErrNotFound)
}

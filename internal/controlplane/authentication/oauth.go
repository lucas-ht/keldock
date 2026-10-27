package authentication

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/lucas-ht/keldock/internal/access"

	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func (service *Service) registerOAuth(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", service.resourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", service.resourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", service.serverMetadata)
	mux.HandleFunc("POST /oauth/register", service.registerPublicClient)
	mux.HandleFunc("POST /oauth/token", service.token)
	mux.HandleFunc("GET /oauth/authorize", service.authorize)
	for _, path := range []string{"/oauth/register", "/oauth/token"} {
		mux.HandleFunc("OPTIONS "+path, func(w http.ResponseWriter, _ *http.Request) {
			oauthCORS(w)
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func oauthCORS(w http.ResponseWriter) { w.Header().Set("Access-Control-Allow-Origin", "*") }

func (service *Service) resourceMetadata(w http.ResponseWriter, _ *http.Request) {
	oauthCORS(w)
	write(w, http.StatusOK, map[string]any{"resource": service.MCP.Resource, "authorization_servers": []string{service.MCP.Issuer}, "scopes_supported": []string{mcpidentity.ReadScope}, "bearer_methods_supported": []string{"header"}, "resource_name": "Keldock read tools"})
}

func (service *Service) serverMetadata(w http.ResponseWriter, _ *http.Request) {
	oauthCORS(w)
	issuer := service.MCP.Issuer
	write(w, http.StatusOK, map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/oauth/authorize", "token_endpoint": issuer + "/oauth/token", "registration_endpoint": issuer + "/oauth/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{mcpidentity.ReadScope}, "authorization_response_iss_parameter_supported": true})
}

func oauthError(w http.ResponseWriter, status int, code string) {
	oauthCORS(w)
	write(w, status, map[string]string{"error": code})
}

func (service *Service) registerPublicClient(w http.ResponseWriter, request *http.Request) {
	oauthCORS(w)
	// Registration grants no data access; a durable budget bounds public registration spam.
	if err := service.Users.AllowLoginAttempt(request.Context(), "oauth-register"); err != nil {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, 16*1024))
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	var input struct {
		Name       string   `json:"client_name"`
		Redirects  []string `json:"redirect_uris"`
		AuthMethod string   `json:"token_endpoint_auth_method"`
		Grants     []string `json:"grant_types"`
		Responses  []string `json:"response_types"`
		Scope      string   `json:"scope"`
	}
	var members map[string]json.RawMessage
	if err := v1.DecodeJSON(body, &members); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	if err := json.Unmarshal(body, &input); err != nil || (input.AuthMethod != "" && input.AuthMethod != "none") || (input.Scope != "" && input.Scope != mcpidentity.ReadScope) || slices.ContainsFunc(input.Grants, func(value string) bool { return value != "authorization_code" && value != "refresh_token" }) || slices.ContainsFunc(input.Responses, func(value string) bool { return value != "code" }) {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	if input.Name == "" {
		input.Name = "MCP client"
	}
	client, err := service.MCP.Register(request.Context(), input.Name, input.Redirects)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	write(w, http.StatusCreated, map[string]any{"client_id": client.ID, "client_name": client.Name, "redirect_uris": client.RedirectURIs, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "scope": mcpidentity.ReadScope})
}

func singleton(values url.Values, keys ...string) bool {
	for _, key := range keys {
		if len(values[key]) > 1 {
			return false
		}
	}
	return true
}

func (service *Service) authorize(w http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	if !singleton(query, "client_id", "redirect_uri", "state", "code_challenge", "code_challenge_method", "resource", "scope", "response_type") {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	id, err := service.MCP.CreateAuthorization(request.Context(), mcpidentity.AuthorizationInput{ClientID: query.Get("client_id"), RedirectURI: query.Get("redirect_uri"), State: query.Get("state"), Challenge: query.Get("code_challenge"), ChallengeMethod: query.Get("code_challenge_method"), Resource: query.Get("resource"), Scope: query.Get("scope"), ResponseType: query.Get("response_type")})
	if errors.Is(err, access.ErrLimited) {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable")
		return
	}
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	http.Redirect(w, request, service.config.PublicURL+"/mcp/authorize?request="+url.QueryEscape(id), http.StatusSeeOther)
}

func (service *Service) token(w http.ResponseWriter, request *http.Request) {
	oauthCORS(w)
	media, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" || request.Header.Get("Authorization") != "" {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, 16*1024)
	if err := request.ParseForm(); err != nil || !singleton(request.PostForm, "client_id", "client_secret", "code", "redirect_uri", "code_verifier", "resource", "refresh_token", "grant_type", "scope") || request.PostForm.Get("client_secret") != "" {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	input := request.PostForm
	if scope := strings.TrimSpace(input.Get("scope")); scope != "" && scope != mcpidentity.ReadScope {
		oauthError(w, http.StatusBadRequest, "invalid_scope")
		return
	}
	var pair mcpidentity.TokenPair
	switch input.Get("grant_type") {
	case "authorization_code":
		pair, err = service.MCP.Exchange(request.Context(), input.Get("client_id"), input.Get("redirect_uri"), input.Get("resource"), input.Get("code"), input.Get("code_verifier"))
	case "refresh_token":
		pair, err = service.MCP.Refresh(request.Context(), input.Get("client_id"), input.Get("resource"), input.Get("refresh_token"))
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	w.Header().Set("Pragma", "no-cache")
	write(w, http.StatusOK, pair)
}

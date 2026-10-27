package authentication

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
)

func (service *Service) mcpSettings(w http.ResponseWriter, request *http.Request) {
	if _, ok := service.human(w, request, false); !ok {
		return
	}
	settings, err := service.MCP.Settings(request.Context())
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, settings)
}

func (service *Service) setMCPSettings(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	var input struct {
		Enabled bool  `json:"enabled"`
		Version int64 `json:"version"`
	}
	if !decode(w, request, &input) {
		return
	}
	settings, err := service.MCP.SetEnabled(request.Context(), principal, input.Enabled, input.Version)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, settings)
}

func (service *Service) mcpClients(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, false)
	if !ok {
		return
	}
	items, next, err := service.MCP.Clients(request.Context(), principal, request.URL.Query().Get("after"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (service *Service) registerMCPClient(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	if !principal.IsAdmin() {
		fail(w, access.ErrDenied)
		return
	}
	var input struct {
		Name      string   `json:"name"`
		Redirects []string `json:"redirectUris"`
	}
	if !decode(w, request, &input) {
		return
	}
	client, err := service.MCP.Register(request.Context(), input.Name, input.Redirects)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, client)
}

func (service *Service) disableMCPClient(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	if err := service.MCP.DisableClient(request.Context(), principal, request.PathValue("client")); err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"state": "disabled"})
}

func (service *Service) connections(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, false)
	if !ok {
		return
	}
	items, next, err := service.MCP.Connections(request.Context(), principal, request.URL.Query().Get("all") == "true", request.URL.Query().Get("after"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (service *Service) revokeConnection(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	if err := service.MCP.RevokeConnection(request.Context(), principal, request.PathValue("connection")); err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"state": "revoked"})
}

func (service *Service) authorization(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, false)
	if !ok {
		return
	}
	info, err := service.MCP.Authorization(request.Context(), principal, request.PathValue("request"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, info)
}

func (service *Service) consent(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	var input struct {
		Allow    bool     `json:"allow"`
		Projects []string `json:"projects"`
	}
	if !decode(w, request, &input) {
		return
	}
	redirect, err := service.MCP.Consent(request.Context(), principal, request.PathValue("request"), input.Allow, input.Projects)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]string{"redirectUrl": redirect})
}

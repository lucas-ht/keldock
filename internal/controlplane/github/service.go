// Package github exposes administrative GitHub onboarding through shared session policy.
package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/controlplane/authentication"
	"github.com/lucas-ht/keldock/internal/githubapp"
	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
)

// Service connects integration administration to the existing human/CSRF boundary.
type Service struct {
	Integration *githubapp.Service
	Security    *authentication.Service
}

// Register advertises only working routes when an installation credential path is configured.
func (service Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/github", service.status)
	mux.HandleFunc("POST /v1/github/app", service.configure)
	mux.HandleFunc("DELETE /v1/github/app", service.disconnect)
	mux.HandleFunc("POST /v1/github/sync", service.sync)
	mux.HandleFunc("POST /v1/github/installations/{installation}/connect", service.installation)
	mux.HandleFunc("POST /v1/github/installations/{installation}/disconnect", service.installation)
}

func (service Service) administrator(w http.ResponseWriter, r *http.Request, mutation bool) (access.Principal, bool) {
	var principal access.Principal
	var err error
	if mutation {
		principal, err = service.Security.Mutation(r)
	} else {
		principal, err = service.Security.Human(r)
	}
	if err != nil || !principal.IsAdmin() {
		failure(w, access.ErrDenied)
		return principal, false
	}
	return principal, true
}

func (service Service) status(w http.ResponseWriter, r *http.Request) {
	if _, ok := service.administrator(w, r, false); !ok {
		return
	}
	state, err := service.Integration.Status(r.Context())
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, state)
}

func (service Service) configure(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int64           `json:"expectedVersion"`
		App             credentials.App `json:"app"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := service.Integration.Configure(r.Context(), principal, input.ExpectedVersion, input.App); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]string{"state": "configured"})
}

func (service Service) disconnect(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := service.Integration.Disconnect(r.Context(), principal, input.ExpectedVersion); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]string{"state": "disconnected"})
}

func (service Service) sync(w http.ResponseWriter, r *http.Request) {
	if _, ok := service.administrator(w, r, true); !ok {
		return
	}
	if err := service.Integration.Sync(r.Context()); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]string{"state": "synchronized"})
}

func (service Service) installation(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("installation"), 10, 64)
	if err != nil || id <= 0 {
		failure(w, access.ErrInvalid)
		return
	}
	var input struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if !decode(w, r, &input) {
		return
	}
	blocked := r.Pattern == "POST /v1/github/installations/{installation}/disconnect"
	if err := service.Integration.Repository.BlockInstallation(r.Context(), principal, input.ExpectedVersion, id, blocked); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]bool{"blocked": blocked})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
	if err == nil {
		err = v1.DecodeJSON(data, target)
	}
	if err != nil {
		failure(w, access.ErrInvalid)
		return false
	}
	return true
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func failure(w http.ResponseWriter, err error) {
	status, code, message := 503, "unavailable", "GitHub integration unavailable; check App configuration and retry"
	switch {
	case errors.Is(err, access.ErrDenied):
		status, code, message = 403, "denied", "Administrator access or GitHub delegation denied"
	case errors.Is(err, access.ErrInvalid):
		status, code, message = 400, "invalid_request", "Invalid GitHub configuration or request"
	case errors.Is(err, access.ErrConflict):
		status, code, message = 409, "conflict", "Configuration changed; refresh before retrying"
	case errors.Is(err, access.ErrNotFound):
		status, code, message = 404, "not_found", "GitHub target not found"
	case errors.Is(err, access.ErrLimited):
		status, code, message = 429, "limited", "Too many pending requests; retry later"
	}
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

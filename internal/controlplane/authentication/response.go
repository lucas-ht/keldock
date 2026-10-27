package authentication

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Debug("identity response interrupted", "error", err)
	}
}

func problem(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, access.ErrDenied):
		problem(w, http.StatusForbidden, "denied", "The request is outside your current access")
	case errors.Is(err, access.ErrNotFound):
		problem(w, http.StatusNotFound, "not_found", "The requested record was not found")
	case errors.Is(err, access.ErrInvalid):
		problem(w, http.StatusBadRequest, "invalid_request", "Check the supplied values and try again")
	case errors.Is(err, access.ErrConflict):
		problem(w, http.StatusConflict, "conflict", "This record changed or the action would remove the last administrator; refresh before retrying")
	case errors.Is(err, access.ErrSessionBusy):
		problem(w, http.StatusConflict, "agent_session_busy", "Another process holds this cluster agent session")
	case errors.Is(err, access.ErrLimited):
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "rate_limited", "Too many authentication attempts; wait before trying again")
	default:
		slog.Error("identity operation unavailable", "error", err)
		problem(w, http.StatusServiceUnavailable, "unavailable", "The control plane is temporarily unavailable; try again")
	}
}

func decode(w http.ResponseWriter, request *http.Request, target any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, 16*1024))
	if err == nil {
		err = v1.DecodeJSON(body, target)
	}
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_request", "Invalid or oversized request")
		return false
	}
	return true
}

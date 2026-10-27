package authentication

import (
	"net/http"

	"github.com/lucas-ht/keldock/internal/access"
)

func (service *Service) users(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, false)
	if !ok {
		return
	}
	items, next, err := service.Users.Users(request.Context(), principal, request.URL.Query().Get("after"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (service *Service) invite(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	var input struct {
		Email    string   `json:"email"`
		Name     string   `json:"name"`
		Role     string   `json:"role"`
		Projects []string `json:"projects"`
	}
	if !decode(w, request, &input) {
		return
	}
	invitation, err := service.Users.Invite(request.Context(), principal, input.Email, input.Name, input.Role, input.Projects)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, service.invitationResponse(invitation))
}

func (service *Service) updateUser(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	var input access.UserUpdate
	if !decode(w, request, &input) {
		return
	}
	user, err := service.Users.UpdateUser(request.Context(), principal, request.PathValue("user"), input)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, user)
}

func (service *Service) reissueInvitation(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	invitation, err := service.Users.ReissueInvitation(request.Context(), principal, request.PathValue("user"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, service.invitationResponse(invitation))
}

func (service *Service) invitationResponse(invitation access.Invitation) map[string]any {
	// A fragment is not sent in HTTP referrers or server request logs.
	return map[string]any{"user": invitation.User, "activationUrl": service.config.PublicURL + "/activate#" + invitation.Token, "expiresAt": invitation.ExpiresAt}
}

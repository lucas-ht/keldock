package authentication

import (
	"net/http"
	"strings"

	"github.com/lucas-ht/keldock/internal/access"
)

// This dummy PHC value burns the same work for unknown accounts; it never authenticates one.
const dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func (service *Service) login(w http.ResponseWriter, request *http.Request) {
	if !service.origin(w, request) {
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, request, &input) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if len(email) > 254 || len(input.Password) > 1024 {
		fail(w, access.ErrInvalid)
		return
	}
	if err := service.Users.AllowLoginAttempt(request.Context(), email); err != nil {
		fail(w, err)
		return
	}
	release, err := service.acquireHash(request.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer release()
	account, lookup := service.Users.PasswordAccount(request.Context(), email)
	hash := account.Hash
	if lookup != nil || hash == "" {
		hash = dummyPasswordHash
	}
	valid := access.VerifyPassword(hash, input.Password)
	if lookup != nil && !IsIdentityError(lookup) {
		fail(w, lookup)
		return
	}
	if !valid || lookup != nil || account.User.Status != "active" {
		problem(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect")
		return
	}
	token, err := service.Users.LoginSession(request.Context(), account.User)
	if err != nil {
		fail(w, err)
		return
	}
	if previous := service.sessionToken(request); previous != "" {
		if err := service.Users.Logout(request.Context(), previous); err != nil {
			fail(w, err)
			return
		}
	}
	service.setSession(w, token)
	write(w, http.StatusOK, map[string]any{"user": account.User, "csrfToken": access.CSRF(token)})
}

func (service *Service) me(w http.ResponseWriter, request *http.Request) {
	principal, expires, err := service.Users.Session(request.Context(), service.sessionToken(request))
	if IsIdentityError(err) {
		problem(w, http.StatusUnauthorized, "not_authenticated", "Sign in to continue")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"user": principal.User, "csrfToken": access.CSRF(service.sessionToken(request)), "sessionExpiresAt": expires})
}

func (service *Service) logout(w http.ResponseWriter, request *http.Request) {
	if _, ok := service.human(w, request, true); !ok {
		return
	}
	if err := service.Users.Logout(request.Context(), service.sessionToken(request)); err != nil {
		fail(w, err)
		return
	}
	service.clearSession(w)
	write(w, http.StatusOK, map[string]string{"state": "signed_out"})
}

func (service *Service) invitationInfo(w http.ResponseWriter, request *http.Request) {
	if !service.origin(w, request) {
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if !decode(w, request, &input) {
		return
	}
	info, kind, err := service.Users.InvitationInfo(request.Context(), input.Token)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"name": info.User.Name, "email": info.User.Email, "kind": kind, "expiresAt": info.ExpiresAt})
}

func (service *Service) activate(w http.ResponseWriter, request *http.Request) {
	if !service.origin(w, request) {
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, request, &input) {
		return
	}
	// Validate the bearer grant before spending password-hash capacity.
	if _, _, err := service.Users.InvitationInfo(request.Context(), input.Token); err != nil {
		fail(w, err)
		return
	}
	release, err := service.acquireHash(request.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer release()
	hash, err := access.HashPassword(input.Password)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_password", "Use a password containing 12–256 characters")
		return
	}
	user, token, err := service.Users.Activate(request.Context(), input.Token, hash)
	if err != nil {
		fail(w, err)
		return
	}
	service.setSession(w, token)
	write(w, http.StatusOK, map[string]any{"user": user, "csrfToken": access.CSRF(token)})
}

func (service *Service) password(w http.ResponseWriter, request *http.Request) {
	principal, ok := service.human(w, request, true)
	if !ok {
		return
	}
	var input struct {
		Current  string `json:"currentPassword"`
		Password string `json:"password"`
	}
	if !decode(w, request, &input) {
		return
	}
	account, err := service.Users.PasswordAccount(request.Context(), principal.User.Email)
	if err != nil {
		fail(w, err)
		return
	}
	release, err := service.acquireHash(request.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer release()
	if !access.VerifyPassword(account.Hash, input.Current) {
		problem(w, http.StatusBadRequest, "invalid_credentials", "Current password is incorrect")
		return
	}
	hash, err := access.HashPassword(input.Password)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_password", "Use a password containing 12–256 characters")
		return
	}
	token, err := service.Users.ChangePassword(request.Context(), principal, account.Hash, hash)
	if err != nil {
		fail(w, err)
		return
	}
	service.setSession(w, token)
	write(w, http.StatusOK, map[string]any{"state": "changed", "csrfToken": access.CSRF(token)})
}

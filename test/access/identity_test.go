//go:build integration

package access_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
)

func TestAccountAuthorityAndCredentialRevocation(t *testing.T) {
	store, repository, ctx := identityDatabase(t)
	admin, adminSession, hash := administrator(t, repository, ctx)
	_, err := repository.UpdateUser(ctx, admin, admin.User.ID, access.UserUpdate{Name: admin.User.Name, Role: "viewer", Status: "active", Version: admin.User.Version})
	if !errors.Is(err, access.ErrConflict) {
		t.Fatalf("last administrator removal: %v", err)
	}
	invitation, err := repository.Invite(ctx, admin, "viewer@example.test", "Project viewer", "viewer", []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	user, session, err := repository.Activate(ctx, invitation.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Activate(ctx, invitation.Token, hash); !errors.Is(err, access.ErrDenied) {
		t.Fatal("single-use invitation replay accepted")
	}
	viewer, _, err := repository.Session(ctx, session)
	if err != nil || !viewer.CanRead("sample") || viewer.CanRead("restricted") || viewer.CanOperate("sample") || viewer.IsAdmin() {
		t.Fatal("viewer authority escaped project/read scope")
	}
	if _, err := repository.Invite(ctx, viewer, "other@example.test", "Other", "admin", nil); !errors.Is(err, access.ErrDenied) {
		t.Fatal("viewer escalated privileges")
	}
	if _, _, err := repository.Users(ctx, viewer, ""); !errors.Is(err, access.ErrDenied) {
		t.Fatal("viewer enumerated users")
	}
	updated, err := repository.UpdateUser(ctx, admin, user.ID, access.UserUpdate{Name: user.Name, Role: "operator", Status: "active", Projects: []string{"sample"}, Version: user.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Session(ctx, session); !errors.Is(err, access.ErrDenied) {
		t.Fatal("old session survived privilege change")
	}
	if _, err := repository.UpdateUser(ctx, admin, user.ID, access.UserUpdate{Name: user.Name, Role: "admin", Status: "active", Version: user.Version}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale administrative update accepted")
	}
	operatorSession, err := repository.LoginSession(ctx, updated)
	if err != nil {
		t.Fatal(err)
	}
	operator, _, err := repository.Session(ctx, operatorSession)
	if err != nil || !operator.CanOperate("sample") || operator.CanOperate("restricted") {
		t.Fatal("new operator scope incorrect")
	}
	reset, err := repository.ReissueInvitation(ctx, admin, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Session(ctx, operatorSession); !errors.Is(err, access.ErrDenied) {
		t.Fatal("password reset did not revoke sessions")
	}
	user, session, err = repository.Activate(ctx, reset.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	user, err = repository.UpdateUser(ctx, admin, user.ID, access.UserUpdate{Name: user.Name, Role: user.Role, Status: "disabled", Projects: user.Projects, Version: user.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Session(ctx, session); !errors.Is(err, access.ErrDenied) {
		t.Fatal("disabled user retained a session")
	}
	if _, err := repository.LoginSession(ctx, user); !errors.Is(err, access.ErrDenied) {
		t.Fatal("disabled user signed in")
	}
	data, err := json.Marshal(user)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), hash) || strings.Contains(string(data), session) {
		t.Fatal("safe metadata exposed a credential")
	}
	var stored string
	if err := store.Pool().QueryRow(ctx, `SELECT token_hash FROM access_sessions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 1`, user.ID).Scan(&stored); err != nil || stored == session || len(stored) != 64 {
		t.Fatal("plaintext session stored")
	}
	if err := repository.Logout(ctx, adminSession); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Session(ctx, adminSession); !errors.Is(err, access.ErrDenied) {
		t.Fatal("logout did not invalidate the session")
	}
}

func TestConcurrentInvitationAndLastAdministrator(t *testing.T) {
	store, repository, ctx := identityDatabase(t)
	admin, _, hash := administrator(t, repository, ctx)
	invitation, err := repository.Invite(ctx, admin, "second@example.test", "Second admin", "admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			if _, _, err := repository.Activate(ctx, invitation.Token, hash); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, access.ErrDenied) {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if successes.Load() != 1 {
		t.Fatalf("invitation accepted %d times", successes.Load())
	}
	second, err := repository.User(ctx, invitation.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondPrincipal := access.Principal{User: second, Kind: "session"}
	successes.Store(0)
	for _, principal := range []access.Principal{admin, secondPrincipal} {
		group.Go(func() {
			_, err := repository.UpdateUser(ctx, principal, principal.User.ID, access.UserUpdate{Name: principal.User.Name, Role: "admin", Status: "disabled", Version: principal.User.Version})
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, access.ErrConflict) {
				t.Error(err)
			}
		})
	}
	group.Wait()
	var active int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM access_users WHERE organization_id=$1 AND role='admin' AND status='active'`, repository.Organization).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 || successes.Load() != 1 {
		t.Fatal("concurrent administrator changes removed the last administrator")
	}
}

func TestSessionExpiryRecoveryAndLoginLimits(t *testing.T) {
	store, repository, ctx := identityDatabase(t)
	admin, session, hash := administrator(t, repository, ctx)
	if _, err := store.Pool().Exec(ctx, `UPDATE access_sessions SET last_used_at=now()-interval '31 minutes' WHERE token_hash=$1`, access.TokenHash(session)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Session(ctx, session); !errors.Is(err, access.ErrDenied) {
		t.Fatal("idle-expired session accepted")
	}
	session, err := repository.LoginSession(ctx, admin.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE access_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, access.TokenHash(session)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Session(ctx, session); !errors.Is(err, access.ErrDenied) {
		t.Fatal("absolute-expired session accepted")
	}
	if err := repository.Recover(ctx, admin.User.Email, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.LoginSession(ctx, admin.User); !errors.Is(err, access.ErrDenied) {
		t.Fatal("pre-recovery account revision authenticated")
	}
	for range 6 {
		if err := repository.AllowLoginAttempt(ctx, admin.User.Email); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.AllowLoginAttempt(ctx, admin.User.Email); !errors.Is(err, access.ErrLimited) {
		t.Fatal("login throttle not enforced")
	}
	if err := repository.ClearLoginLimit(ctx, admin.User.Email); err != nil {
		t.Fatal(err)
	}
	if err := repository.AllowLoginAttempt(ctx, admin.User.Email); err != nil {
		t.Fatal(err)
	}
}

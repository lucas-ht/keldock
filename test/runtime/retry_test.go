//go:build integration

package runtime_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/inspection"
)

func TestHumanRetryKeepsAdmittedInputsAndGenerationConcurrency(t *testing.T) {
	fixture := newAPI(t)
	input := fixture.sign(t, applicationInput(t))
	receipt := fixture.submit(t, input, "retry-original-release", http.StatusAccepted)
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/cancel", fixture.userHeaders(), nil, nil, http.StatusAccepted)
	headers := fixture.userHeaders()
	headers["Idempotency-Key"] = "same-release-retry"
	var retried, replayed v1.Receipt
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/retry", headers, map[string]int64{"generation": receipt.Generation}, &retried, http.StatusAccepted)
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/retry", headers, map[string]int64{"generation": receipt.Generation}, &replayed, http.StatusAccepted)
	if retried != replayed || retried.ReleaseID != receipt.ReleaseID || retried.Generation != receipt.Generation+1 || retried.OperationID == receipt.OperationID {
		t.Fatal("retry changed immutable inputs or lost receipt/generation guarantees")
	}
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/retry", headers, map[string]int64{"generation": retried.Generation}, nil, http.StatusConflict)
	headers["Idempotency-Key"] = "competing-retry-request"
	fixture.call(t, http.MethodPost, receipt.StatusURL+"/retry", headers, map[string]int64{"generation": receipt.Generation}, nil, http.StatusConflict)
	fixture.call(t, http.MethodPost, retried.StatusURL+"/retry", headers, map[string]any{"generation": retried.Generation, "images": input.Images}, nil, http.StatusBadRequest)
	fixture.call(t, http.MethodGet, "/v1/projects/other/environments/"+receipt.EnvironmentID, fixture.userHeaders(), nil, nil, http.StatusForbidden)
	var detail inspection.Environment
	fixture.call(t, http.MethodGet, "/v1/projects/sample/environments/"+receipt.EnvironmentID, fixture.userHeaders(), nil, &detail, http.StatusOK)
	if detail.Source == nil || detail.Source.RunID != input.Source.RunID || detail.Source.RunAttempt != input.Source.RunAttempt || detail.Generation != retried.Generation {
		t.Fatal("retry forged a new CI source identity")
	}
	ctx := context.Background()
	principal, _, err := fixture.api.Security.Users.Session(ctx, fixture.session)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := fixture.api.Security.Users.Invite(ctx, principal, "viewer@example.test", "Read only", "viewer", []string{"sample"})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := access.HashPassword("synthetic viewer test password")
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := fixture.api.Security.Users.Activate(ctx, invitation.Token, hash)
	if err != nil {
		t.Fatal(err)
	}
	viewerHeaders := map[string]string{"Cookie": fixture.api.Security.CookieName() + "=" + session, "Origin": fixture.server.URL, "X-Keldock-CSRF": access.CSRF(session), "Idempotency-Key": "viewer-retry-request"}
	fixture.call(t, http.MethodPost, retried.StatusURL+"/cancel", viewerHeaders, nil, nil, http.StatusForbidden)
	fixture.call(t, http.MethodPost, retried.StatusURL+"/retry", viewerHeaders, map[string]int64{"generation": retried.Generation}, nil, http.StatusForbidden)
	fixture.call(t, http.MethodPost, "/v1/users", viewerHeaders, map[string]any{"email": "other@example.test", "name": "Other", "role": "admin", "projects": []string{}}, nil, http.StatusForbidden)
	fixture.call(t, http.MethodGet, retried.StatusURL, viewerHeaders, nil, nil, http.StatusOK)
}

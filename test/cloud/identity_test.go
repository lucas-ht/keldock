//go:build integration

package cloud_test

import (
	"errors"
	"testing"

	"github.com/lucas-ht/keldock/internal/access"
	"github.com/lucas-ht/keldock/internal/access/clusteridentity"
)

func TestCloudAgentAuthorityIsIndependent(t *testing.T) {
	ctx, workloads, store, principal := database(t)
	registrar, err := clusteridentity.NewRepository(workloads.Pool(), store.Users, "keldock-m1")
	if err != nil {
		t.Fatal(err)
	}
	provisioner, err := clusteridentity.NewCloudRepository(workloads.Pool(), store.Users, "keldock-m1")
	if err != nil {
		t.Fatal(err)
	}
	for _, repository := range []*clusteridentity.Repository{registrar, provisioner} {
		if err := repository.Seed(ctx, "Fixture"); err != nil {
			t.Fatal(err)
		}
	}
	workGrant, err := registrar.CreateEnrollment(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	cloudGrant, err := provisioner.CreateEnrollment(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provisioner.Exchange(ctx, "keldock-m1", workGrant.Token); !errors.Is(err, access.ErrDenied) {
		t.Fatal("workload enrollment accepted by cloud audience")
	}
	workIdentity, err := registrar.Exchange(ctx, "keldock-m1", workGrant.Token)
	if err != nil {
		t.Fatal(err)
	}
	cloudIdentity, err := provisioner.Exchange(ctx, "keldock-m1", cloudGrant.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		repository *clusteridentity.Repository
		token      string
	}{{provisioner, workIdentity.Token}, {registrar, cloudIdentity.Token}} {
		if _, err := pair.repository.Authenticate(ctx, pair.token, "fixture-process-session", "test"); !errors.Is(err, access.ErrDenied) {
			t.Fatal("credential crossed provisioning audience")
		}
	}
	if _, err := provisioner.Authenticate(ctx, cloudIdentity.Token, "fixture-process-session", "test"); err != nil {
		t.Fatal(err)
	}
	if err := provisioner.Revoke(ctx, principal); err != nil {
		t.Fatal(err)
	}
	if _, err := provisioner.Authenticate(ctx, cloudIdentity.Token, "fixture-process-session", "test"); !errors.Is(err, access.ErrDenied) {
		t.Fatal("revoked provisioning credential remained valid")
	}
	if _, err := registrar.Authenticate(ctx, workIdentity.Token, "fixture-process-session", "test"); err != nil {
		t.Fatal("cloud revocation disturbed registrar", err)
	}
}

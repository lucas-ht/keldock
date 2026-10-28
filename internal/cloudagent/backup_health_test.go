package cloudagent

import (
	"context"
	"testing"

	"github.com/lucas-ht/keldock/internal/cloud"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestBackupWaitsForExactArchiveRBACBeforeRequestingAWSCredentials(t *testing.T) {
	kube := fake.NewClientset()
	calls := 0
	kube.PrependReactor("create", "subjectaccessreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
		calls++
		review := action.(ktesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
		a := review.Spec.ResourceAttributes
		if review.Spec.User != "system:serviceaccount:database:source" || a.Namespace != "database" || a.Group != "barmancloud.cnpg.io" || a.Resource != "objectstores" || a.Name != "archive-generation-2" || a.Verb != "get" {
			t.Fatal("backup readiness queried a different authority")
		}
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: false}}, nil
	})
	// No AWS client exists: a denied RBAC gate must return without any token exchange.
	driver := &Driver{Kubernetes: kube}
	ready, err := driver.backupAuthorityReady(context.Background(), cloud.Policy{}, cloud.Connection{}, cloud.BackupBinding{Database: cloud.DatabaseTarget{Namespace: "database", Name: "source"}}, "bucket", "archive-generation-2")
	if err != nil || ready || calls != 1 {
		t.Fatal("backup ignored missing archive RBAC", err)
	}
}

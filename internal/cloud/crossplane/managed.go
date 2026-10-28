package crossplane

import (
	"crypto/sha256"
	"fmt"

	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Name isolates identical caller-selected IDs across projects and managed resource kinds.
func Name(scope v1.Scope, id, kind string) string {
	return "kc-" + fmt.Sprintf("%x", sha256.Sum256([]byte(scope.OrganizationID+"/"+scope.ProjectID+"/"+id+"/"+kind)))[:24]
}

// ConfigName resolves a separate provider role/config for each connection and service.
func ConfigName(connection cloud.Connection, service string) string {
	return Name(connection.Scope, connection.ID, "config-"+service)
}

// ProviderConfigs select only short-lived WebIdentity credentials from the pinned pod projection.
func ProviderConfigs(policy cloud.Policy, connection cloud.Connection) ([]map[string]any, error) {
	if err := policy.ValidateConnection(connection); err != nil {
		return nil, err
	}
	identity := policy.Identities[connection.IdentityRef]
	result := []map[string]any{}
	for _, service := range []string{"s3", "iam", "route53"} {
		result = append(result, map[string]any{"apiVersion": "aws.m.upbound.io/v1beta1", "kind": "ProviderConfig", "metadata": map[string]any{"name": ConfigName(connection, service), "namespace": identity.Namespace, "labels": owner(connection.Scope, connection.ID)}, "spec": map[string]any{"credentials": map[string]any{"source": "WebIdentity", "webIdentity": map[string]any{"roleARN": identity.Roles[service], "tokenConfig": map[string]any{"source": "Filesystem", "fs": map[string]string{"path": "/var/run/keldock/aws/token"}}}}, "skip_metadata_api_check": true}})
	}
	return result, nil
}

func owner(scope v1.Scope, id string) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "keldock-cloud", "keldock.io/organization": scope.OrganizationID, "keldock.io/project": scope.ProjectID, "keldock.io/cloud-intent": id}
}

func managed(policy cloud.Policy, connection cloud.Connection, id, kind, group, service, external string, parameters map[string]any, observe bool) map[string]any {
	policies := []string{"Observe", "Create", "Update"}
	if observe {
		policies = []string{"Observe"}
	}
	return map[string]any{"apiVersion": group + "/v1beta1", "kind": kind, "metadata": map[string]any{"name": Name(connection.Scope, id, kind), "namespace": policy.Identities[connection.IdentityRef].Namespace, "labels": owner(connection.Scope, id), "annotations": map[string]string{"crossplane.io/external-name": external}}, "spec": map[string]any{"managementPolicies": policies, "providerConfigRef": map[string]string{"name": ConfigName(connection, service), "kind": "ProviderConfig"}, "forProvider": parameters}}
}

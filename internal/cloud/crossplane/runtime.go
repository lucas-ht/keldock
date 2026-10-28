// Package crossplane renders the fixed optional AWS implementation, never arbitrary provider code.
package crossplane

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/lucas-ht/keldock/internal/cloud"
)

//go:embed controllers.lock.json
var pinsJSON []byte

// Pins records anonymously verified community distribution digests and controller artifacts.
type Pins struct {
	Packages  map[string]string          `json:"packages"`
	Images    map[string]string          `json:"images"`
	Artifacts map[string]json.RawMessage `json:"artifacts"`
}

// Locked returns the shared, checked-in artifact lock used by the explicit fixture installer.
func Locked() (Pins, error) { var pins Pins; err := json.Unmarshal(pinsJSON, &pins); return pins, err }

// Runtime uses pre-created service accounts without transferring their ownership to package revisions.
func Runtime(policy cloud.Policy) ([]map[string]any, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	pins, err := Locked()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(policy.Identities))
	for name := range policy.Identities {
		names = append(names, name)
	}
	slices.Sort(names)
	identity := policy.Identities[names[0]]
	objects := []map[string]any{}
	for _, service := range []string{"s3", "iam", "route53"} {
		sa := identity.ServiceAccounts[service]
		objects = append(objects, map[string]any{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": sa, "namespace": identity.Namespace}, "automountServiceAccountToken": true})
		runtime := map[string]any{"apiVersion": "pkg.crossplane.io/v1beta1", "kind": "DeploymentRuntimeConfig", "metadata": map[string]string{"name": "keldock-aws-" + service}, "spec": map[string]any{"deploymentTemplate": map[string]any{"spec": map[string]any{"selector": map[string]any{}, "template": map[string]any{"spec": map[string]any{
			"serviceAccountName": sa, "securityContext": map[string]any{"runAsNonRoot": true, "fsGroup": 65532},
			"containers": []any{map[string]any{"name": "package-runtime", "env": []any{map[string]string{"name": "AWS_REGION", "value": policy.Regions[0]}, map[string]string{"name": "AWS_STS_REGIONAL_ENDPOINTS", "value": "regional"}, map[string]string{"name": "AWS_EC2_METADATA_DISABLED", "value": "true"}}, "volumeMounts": []any{map[string]any{"name": "aws-identity", "mountPath": "/var/run/keldock/aws", "readOnly": true}}, "resources": map[string]any{"requests": map[string]string{"cpu": "100m", "memory": "128Mi"}, "limits": map[string]string{"cpu": "1", "memory": "768Mi"}}}},
			"volumes":    []any{map[string]any{"name": "aws-identity", "projected": map[string]any{"defaultMode": 288, "sources": []any{map[string]any{"serviceAccountToken": map[string]any{"audience": "sts.amazonaws.com", "expirationSeconds": 900, "path": "token"}}}}}},
		}}}}}}
		objects = append(objects, runtime)
	}
	for _, name := range []string{"provider-family-aws", "provider-aws-s3", "provider-aws-iam", "provider-aws-route53"} {
		ref, ok := pins.Packages[name]
		if !ok {
			return nil, fmt.Errorf("missing community provider pin")
		}
		spec := map[string]any{"package": ref, "skipDependencyResolution": true, "revisionHistoryLimit": 1, "packagePullPolicy": "IfNotPresent"}
		if name != "provider-family-aws" {
			spec["runtimeConfigRef"] = map[string]string{"name": "keldock-aws-" + name[len("provider-aws-"):]}
		}
		objects = append(objects, map[string]any{"apiVersion": "pkg.crossplane.io/v1", "kind": "Provider", "metadata": map[string]string{"name": name}, "spec": spec})
	}
	return objects, nil
}

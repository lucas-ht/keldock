package crossplane

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/lucas-ht/keldock/internal/cloud"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Grant is resolved from an approved retained resource and one exact consumer prefix.
type Grant struct{ Bucket, Prefix, Access string }

// ConsumerRole is the union of one subject's currently approved bindings.
type ConsumerRole struct {
	ID             string
	ARN            string
	Namespace      string
	ServiceAccount string
	Profile        string
	Grants         []Grant
}

// Role owns one generated role and its single union policy; no second writer manages inline policies.
func Role(policy cloud.Policy, connection cloud.Connection, role ConsumerRole) (map[string]any, string, string, error) {
	if err := policy.ValidateConnection(connection); err != nil {
		return nil, "", "", err
	}
	profile, ok := policy.Profiles[role.Profile]
	if !ok || (profile.Kind != cloud.ObjectStorage && profile.Kind != cloud.BackupStore) || !v1.ValidName(role.Namespace) || !v1.ValidName(role.ServiceAccount) || !strings.HasPrefix(role.ARN, "arn:aws:iam::"+connection.Account+":role"+profile.RolePath) {
		return nil, "", "", fmt.Errorf("consumer role escaped approved path")
	}
	identity := policy.Identities[connection.IdentityRef]
	issuer := strings.TrimPrefix(identity.Issuer, "https://")
	trust := map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": "sts:AssumeRoleWithWebIdentity", "Principal": map[string]string{"Federated": "arn:aws:iam::" + connection.Account + ":oidc-provider/" + issuer}, "Condition": map[string]any{"StringEquals": map[string]string{issuer + ":aud": "sts.amazonaws.com", issuer + ":sub": "system:serviceaccount:" + role.Namespace + ":" + role.ServiceAccount}}}}}
	statements := []any{}
	slices.SortFunc(role.Grants, func(a, b Grant) int {
		return strings.Compare(a.Bucket+"/"+a.Prefix+"/"+a.Access, b.Bucket+"/"+b.Prefix+"/"+b.Access)
	})
	for _, grant := range role.Grants {
		if (!strings.HasPrefix(grant.Bucket, profile.BucketPrefix) && !slices.Contains(profile.ExistingBuckets, grant.Bucket)) || grant.Prefix == "" || !strings.HasSuffix(grant.Prefix, "/") || strings.ContainsAny(grant.Prefix, "*?\\") || (grant.Access != "read" && grant.Access != "read-write") {
			return nil, "", "", fmt.Errorf("consumer grant escaped approved bucket and prefix")
		}
		bucket := "arn:aws:s3:::" + grant.Bucket
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:ListBucket", "s3:ListBucketVersions"}, "Resource": []string{bucket}, "Condition": map[string]any{"StringLike": map[string]any{"s3:prefix": []string{grant.Prefix, grant.Prefix + "*"}}}})
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetBucketLocation"}, "Resource": []string{bucket}})
		actions := []string{"s3:GetObject", "s3:GetObjectVersion"}
		if grant.Access == "read-write" {
			actions = append(actions, "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts")
		}
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{bucket + "/" + grant.Prefix + "*"}})
	}
	if len(statements) == 0 {
		statements = append(statements, map[string]any{"Effect": "Deny", "Action": []string{"s3:*"}, "Resource": []string{"arn:aws:s3:::" + profile.BucketPrefix + "*", "arn:aws:s3:::" + profile.BucketPrefix + "*/*"}})
	}
	for _, raw := range statements {
		statement := raw.(map[string]any)
		if statement["Effect"] != "Allow" {
			continue
		}
		condition, _ := statement["Condition"].(map[string]any)
		if condition == nil {
			condition = map[string]any{}
		}
		condition["StringEquals"] = map[string]string{"aws:ResourceAccount": connection.Account}
		statement["Condition"] = condition
	}
	permissions := map[string]any{"Version": "2012-10-17", "Statement": statements}
	trustJSON, _ := json.Marshal(trust)
	policyJSON, _ := json.Marshal(permissions)
	name := role.ARN[strings.LastIndex(role.ARN, "/")+1:]
	parameters := map[string]any{"path": profile.RolePath, "assumeRolePolicy": string(trustJSON), "permissionsBoundary": profile.BoundaryARN, "maxSessionDuration": 3600, "forceDetachPolicies": false, "inlinePolicy": []any{map[string]string{"name": "keldock", "policy": string(policyJSON)}}}
	return managed(policy, connection, role.ID, "Role", "iam.aws.m.upbound.io", "iam", name, parameters, false), string(trustJSON), string(policyJSON), nil
}

package bootstrap

import "github.com/lucas-ht/keldock/internal/cloud"

func storagePolicy(spec Spec) map[string]any {
	bucket := "arn:aws:s3:::" + spec.DataPrefix() + "*"
	locationKey := "StringEquals"
	if spec.Region == "us-east-1" {
		locationKey = "StringEqualsIfExists"
	}
	return document([]any{
		statement("Allow", []string{"s3:CreateBucket"}, []string{bucket}, map[string]any{"Condition": map[string]any{locationKey: map[string]string{"s3:LocationConstraint": spec.Region}}}),
		statement("Allow", []string{"s3:GetBucket*", "s3:GetAccelerateConfiguration", "s3:GetEncryptionConfiguration", "s3:GetLifecycleConfiguration", "s3:GetReplicationConfiguration", "s3:ListBucket", "s3:ListBucketVersions", "s3:PutBucketVersioning", "s3:PutEncryptionConfiguration", "s3:PutBucketPublicAccessBlock", "s3:PutBucketOwnershipControls"}, []string{bucket}, nil),
	})
}

func identityPolicy(spec Spec) map[string]any {
	roles := "arn:aws:iam::" + spec.Account + ":role" + spec.ConsumerPath() + "*"
	return document([]any{
		statement("Allow", []string{"iam:GetRole"}, []string{"arn:aws:iam::" + spec.Account + ":role/" + cloud.ConsumerRolePrefix(cloud.Profile{RolePath: spec.ConsumerPath()}) + "*"}, nil),
		statement("Allow", []string{"iam:CreateRole", "iam:PutRolePermissionsBoundary"}, []string{roles}, map[string]any{"Condition": map[string]any{"StringEquals": map[string]string{"iam:PermissionsBoundary": spec.BoundaryARN()}}}),
		statement("Allow", []string{"iam:GetRole", "iam:GetRolePolicy", "iam:ListRolePolicies", "iam:ListRoleTags", "iam:ListAttachedRolePolicies", "iam:ListInstanceProfilesForRole", "iam:UpdateRole", "iam:UpdateAssumeRolePolicy", "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:TagRole", "iam:UntagRole", "iam:DeleteRole"}, []string{roles}, nil),
		statement("Allow", []string{"iam:GetPolicy", "iam:GetPolicyVersion", "iam:ListPolicyVersions"}, []string{spec.BoundaryARN()}, nil),
	})
}

func consumerBoundary(spec Spec) map[string]any {
	bucket := "arn:aws:s3:::" + spec.DataPrefix() + "*"
	return document([]any{
		statement("Allow", []string{"s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketVersions"}, []string{bucket}, nil),
		statement("Allow", []string{"s3:GetObject", "s3:GetObjectVersion", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"}, []string{bucket + "/*"}, nil),
	})
}

func dnsPolicy(spec Spec, challengeOnly bool) map[string]any {
	foundationName, _ := spec.Names()
	zone := map[string]any{"Fn::Sub": []any{"arn:aws:route53:::hostedzone/${ZoneID}", map[string]any{"ZoneID": map[string]string{"Fn::ImportValue": foundationName + "-zone-id"}}}}
	types := []string{"A", "AAAA", "CNAME", "TXT"}
	if challengeOnly {
		types = []string{"TXT"}
	}
	return document([]any{
		statement("Allow", []string{"route53:GetHostedZone", "route53:ListResourceRecordSets"}, []any{zone}, nil),
		statement("Allow", []string{"route53:ChangeResourceRecordSets"}, []any{zone}, map[string]any{"Condition": map[string]any{"ForAllValues:StringEquals": map[string]any{"route53:ChangeResourceRecordSetsRecordTypes": types, "route53:ChangeResourceRecordSetsActions": []string{"CREATE", "UPSERT", "DELETE"}}}}),
		statement("Allow", []string{"route53:GetChange"}, []string{"arn:aws:route53:::change/*"}, nil),
	})
}

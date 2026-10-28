package bootstrap

import "strings"

func identityStack(spec Spec) map[string]any {
	issuer := strings.TrimPrefix(spec.Issuer(), "https://")
	resources := map[string]any{
		"ClusterIssuer": retained("AWS::IAM::OIDCProvider", map[string]any{"Url": spec.Issuer(), "ClientIdList": []string{"sts.amazonaws.com"}, "Tags": tags(spec)}),
		"ConsumerBoundary": retained("AWS::IAM::ManagedPolicy", map[string]any{
			"ManagedPolicyName": "keldock-" + spec.InstallationID + "-consumer-boundary", "Path": "/keldock/" + spec.InstallationID + "/", "PolicyDocument": consumerBoundary(spec),
		}),
	}
	for _, service := range []string{"s3", "iam", "route53", "dns01"} {
		namespace, subject := "keldock-cloud", "provider-"+service
		if service == "dns01" {
			namespace = "cert-manager"
			subject = "cert-manager"
		}
		trust := map[string]any{"Effect": "Allow", "Principal": map[string]any{"Federated": map[string]string{"Ref": "ClusterIssuer"}}, "Action": "sts:AssumeRoleWithWebIdentity", "Condition": map[string]any{"StringEquals": map[string]string{issuer + ":aud": "sts.amazonaws.com", issuer + ":sub": "system:serviceaccount:" + namespace + ":" + subject}}}
		name := map[string]string{"s3": "StorageRole", "iam": "IdentityRole", "route53": "DNSRole", "dns01": "DNSChallengeRole"}[service]
		properties := map[string]any{
			"RoleName": "keldock-" + spec.InstallationID + "-" + service, "Path": "/keldock/" + spec.InstallationID + "/providers/", "MaxSessionDuration": 3600,
			"AssumeRolePolicyDocument": map[string]any{"Version": "2012-10-17", "Statement": []any{trust}},
			"Policies":                 []any{map[string]any{"PolicyName": "keldock-scoped", "PolicyDocument": providerPolicy(spec, service)}}, "Tags": tags(spec),
		}
		resources[name] = retained("AWS::IAM::Role", properties)
	}
	return map[string]any{"AWSTemplateFormatVersion": "2010-09-09", "Description": "Keldock exact service-account trust and bounded cloud roles", "Resources": resources,
		"Outputs": map[string]any{
			"OIDCProviderARN":     map[string]any{"Value": map[string]string{"Ref": "ClusterIssuer"}},
			"ConsumerBoundaryARN": map[string]any{"Value": map[string]string{"Ref": "ConsumerBoundary"}},
			"StorageRoleARN":      map[string]any{"Value": map[string]any{"Fn::GetAtt": []string{"StorageRole", "Arn"}}},
			"IdentityRoleARN":     map[string]any{"Value": map[string]any{"Fn::GetAtt": []string{"IdentityRole", "Arn"}}},
			"DNSRoleARN":          map[string]any{"Value": map[string]any{"Fn::GetAtt": []string{"DNSRole", "Arn"}}},
			"DNSChallengeRoleARN": map[string]any{"Value": map[string]any{"Fn::GetAtt": []string{"DNSChallengeRole", "Arn"}}},
		},
	}
}

func providerPolicy(spec Spec, service string) map[string]any {
	switch service {
	case "s3":
		return storagePolicy(spec)
	case "iam":
		return identityPolicy(spec)
	default:
		return dnsPolicy(spec, service == "dns01")
	}
}

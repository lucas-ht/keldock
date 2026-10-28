package bootstrap

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderAuthorityCannotModifyTrustRootsOrReadData(t *testing.T) {
	spec := testSpec()
	storage, _ := json.Marshal(storagePolicy(spec))
	for _, forbidden := range []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", spec.IssuerBucket(), `"Resource":"*"`} {
		if strings.Contains(string(storage), forbidden) {
			t.Fatalf("storage provider has forbidden authority %s", forbidden)
		}
	}
	identity, _ := json.Marshal(identityPolicy(spec))
	for _, forbidden := range []string{"iam:PassRole", "iam:DeleteRolePermissionsBoundary", "iam:AttachRolePolicy", "/providers/", "iam:CreatePolicy", `"Resource":"*"`} {
		if strings.Contains(string(identity), forbidden) {
			t.Fatalf("identity provider has forbidden authority %s", forbidden)
		}
	}
	if !strings.Contains(string(identity), `"iam:PermissionsBoundary":"`+spec.BoundaryARN()+`"`) || !strings.Contains(string(identity), ":role"+spec.ConsumerPath()+"*") {
		t.Fatal("created roles lack mandatory boundary and path")
	}
	for _, challengeOnly := range []bool{false, true} {
		dns, _ := json.Marshal(dnsPolicy(spec, challengeOnly))
		if strings.Contains(string(dns), spec.ParentZoneID) || strings.Contains(string(dns), "DeleteHostedZone") || strings.Contains(string(dns), "CreateHostedZone") {
			t.Fatal("DNS provider owns parent or hosted-zone lifecycle")
		}
		if challengeOnly && (strings.Contains(string(dns), `"A"`) || strings.Contains(string(dns), `"CNAME"`)) {
			t.Fatal("challenge solver can change application routing")
		}
	}
}

func TestExactServiceAccountTrust(t *testing.T) {
	spec := testSpec()
	resources := identityStack(spec)["Resources"].(map[string]any)
	for key, subject := range map[string]string{"StorageRole": "keldock-cloud:provider-s3", "IdentityRole": "keldock-cloud:provider-iam", "DNSRole": "keldock-cloud:provider-route53", "DNSChallengeRole": "cert-manager:cert-manager"} {
		role := resources[key].(map[string]any)["Properties"].(map[string]any)
		trust, _ := json.Marshal(role["AssumeRolePolicyDocument"])
		for _, required := range []string{spec.Issuer()[8:] + ":aud", "sts.amazonaws.com", "system:serviceaccount:" + subject, "StringEquals"} {
			if !strings.Contains(string(trust), required) {
				t.Fatalf("%s missing exact trust constraint %s", key, required)
			}
		}
		if strings.Contains(string(trust), "*") {
			t.Fatalf("%s has wildcard trust", key)
		}
	}
}

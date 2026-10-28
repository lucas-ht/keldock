package awsprobe

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestProjectedIdentityMustMatchOperatorIssuerAudienceAndSubject(t *testing.T) {
	for _, tc := range []struct {
		name, issuer, subject string
		audience              []string
		allowed               bool
	}{
		{"approved", "https://issuer.example.test", "system:serviceaccount:cloud:provider-s3", []string{"sts.amazonaws.com"}, true},
		{"other issuer", "https://other.example.test", "system:serviceaccount:cloud:provider-s3", []string{"sts.amazonaws.com"}, false},
		{"other namespace", "https://issuer.example.test", "system:serviceaccount:production:provider-s3", []string{"sts.amazonaws.com"}, false},
		{"other subject", "https://issuer.example.test", "system:serviceaccount:cloud:admin", []string{"sts.amazonaws.com"}, false},
		{"kubernetes audience", "https://issuer.example.test", "system:serviceaccount:cloud:provider-s3", []string{"https://kubernetes.default.svc"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]any{"iss": tc.issuer, "sub": tc.subject, "aud": tc.audience})
			token := "header." + base64.RawURLEncoding.EncodeToString(b) + ".signature"
			if expectedToken(token, "https://issuer.example.test", "cloud", "provider-s3") != tc.allowed {
				t.Fatal("unexpected identity acceptance")
			}
		})
	}
}

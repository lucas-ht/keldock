package compiler

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
)

func TestCloudBindingIsExactAndDoesNotLeakToOtherComponents(t *testing.T) {
	input, selection := fixture()
	input.Definition.Components["web"] = func() v1.Component {
		c := input.Definition.Components["web"]
		c.Env = map[string]string{"AWS_ROLE_ARN": "source-cannot-choose-role", "AWS_ACCESS_KEY_ID": "source-cannot-override-trusted-identity", "NORMAL": "retained"}
		return c
	}()
	selection.Cloud = map[string]v1.WorkloadCloud{"web": {ConnectionID: "aws-test", ConnectionGeneration: 1, ServiceAccount: "wk-web", RoleARN: "arn:aws:iam::123456789012:role/keldock/test/consumers/web", Region: "eu-central-1", EgressCIDRs: []string{"3.120.1.2/32"}, Stores: map[string]v1.ObjectStoreBinding{"store-one": {Bucket: "keldock-test-bucket", Prefix: "environments/one/web/", Access: "read-write"}}}}
	bundle, err := Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	var deployment appsv1.Deployment
	if err := json.Unmarshal(bundle.Files["components/web/object-0.json"], &deployment); err != nil {
		t.Fatal(err)
	}
	pod := deployment.Spec.Template.Spec
	if pod.ServiceAccountName != "wk-web" || pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatal("binding changed API-token isolation or selected wrong subject")
	}
	env := map[string]string{}
	for _, value := range pod.Containers[0].Env {
		env[value.Name] = value.Value
	}
	if env["AWS_ROLE_ARN"] != selection.Cloud["web"].RoleARN || env["AWS_ACCESS_KEY_ID"] != "" || env["AWS_EC2_METADATA_DISABLED"] != "true" || env["NORMAL"] != "retained" {
		t.Fatal("source could override trusted cloud identity")
	}
	if len(pod.Volumes) != 2 || pod.Volumes[1].Projected.Sources[0].ServiceAccountToken.Audience != "sts.amazonaws.com" {
		t.Fatal("AWS projection has wrong audience or leaked the Kubernetes API token")
	}
	if strings.Contains(string(bundle.Files["components/api/object-0.json"]), "AWS_ROLE_ARN") {
		t.Fatal("cloud binding leaked to unapproved component")
	}
	policy := networkPolicy("web", input.Definition, selection)
	last := policy.Spec.Egress[len(policy.Spec.Egress)-1]
	if len(last.To) != 1 || last.To[0].IPBlock.CIDR != "3.120.1.2/32" || last.Ports[0].Port.IntVal != 443 {
		t.Fatal("cloud egress expanded beyond approved HTTPS addresses")
	}
}

func TestCustomDomainCertificateHasOneApprovedOwnerAndBoundedNames(t *testing.T) {
	input, selection := fixture()
	selection.Domains = []v1.DomainRoute{{ID: strings.Repeat("a", 40), Component: "web", Hostname: "app.test.example.com", IssuerRef: "keldock-dns01"}}
	bundle, err := Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	certificates := 0
	for path, data := range bundle.Files {
		var object map[string]any
		if json.Unmarshal(data, &object) != nil {
			t.Fatal("invalid compiled object")
		}
		if object["kind"] != "Certificate" {
			continue
		}
		certificates++
		if !strings.HasPrefix(path, "components/web/") {
			t.Fatal("certificate allocated to another consumer")
		}
		metadata := object["metadata"].(map[string]any)
		if len(metadata["name"].(string)) > 63 || metadata["namespace"] != selection.Namespace {
			t.Fatal("certificate escaped namespace or name limit")
		}
		spec := object["spec"].(map[string]any)
		if spec["issuerRef"].(map[string]any)["name"] != "keldock-dns01" {
			t.Fatal("source selected a different issuer")
		}
	}
	if certificates != 1 {
		t.Fatal("custom route did not produce exactly one certificate")
	}
}

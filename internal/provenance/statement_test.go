package provenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

func TestVerifiedReleaseEvidence(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: "sample", Definition: v1.Application{
		APIVersion: v1.APIVersion, Name: "sample", Components: map[string]v1.Component{"web": {Kind: "http", Size: "small", Port: 8080, Probe: &v1.Probe{Path: "/ready"}}}},
		Source: v1.Source{RepositoryID: 1, OwnerID: 2, Repository: "example/app", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main", Event: "push", WorkflowRef: "example/app/.github/workflows/build.yaml@refs/heads/main", RunID: 3, RunAttempt: 1},
		Images: map[string]string{"web": "registry.example/app@sha256:" + strings.Repeat("b", 64)}}
	statement, err := NewStatement(input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := Sign(body, "build-key", private)
	if err != nil {
		t.Fatal(err)
	}
	input.Provenance, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"build-key": public}
	if err := VerifyRelease(input, keys); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*v1.SubmitRelease)
	}{
		{"changed digest", func(r *v1.SubmitRelease) {
			r.Images = map[string]string{"web": "registry.example/app@sha256:" + strings.Repeat("c", 64)}
		}},
		{"changed registry", func(r *v1.SubmitRelease) {
			r.Images = map[string]string{"web": "evil.example/app@sha256:" + strings.Repeat("b", 64)}
		}},
		{"changed definition", func(r *v1.SubmitRelease) { r.Definition.Name = "other" }},
		{"different repository", func(r *v1.SubmitRelease) { r.Source.RepositoryID = 5 }},
		{"different run", func(r *v1.SubmitRelease) { r.Source.RunID = 5 }},
		{"different workflow", func(r *v1.SubmitRelease) { r.Source.WorkflowRef = "other/workflow" }},
		{"different project", func(r *v1.SubmitRelease) { r.ProjectID = "other" }},
		{"unsigned", func(r *v1.SubmitRelease) {
			r.Provenance = json.RawMessage(`{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := input
			test.change(&changed)
			if err := VerifyRelease(changed, keys); err == nil {
				t.Fatal("changed release admitted with original evidence")
			}
		})
	}
	if err := VerifyRelease(input, map[string]ed25519.PublicKey{}); err == nil {
		t.Fatal("unapproved signing key accepted")
	}
}

func TestDSSEProtocol(t *testing.T) {
	if got := string(pae([]byte("hello"))); got != "DSSEv1 28 application/vnd.in-toto+json 5 hello" {
		t.Fatalf("incorrect pre-authentication encoding: %s", got)
	}
	if _, err := Sign(nil, "key", nil); err == nil {
		t.Fatal("invalid key accepted")
	}
	for _, data := range []string{`{`, `{}`, `{"payloadType":"wrong"}`, `{"payloadType":"application/vnd.in-toto+json","payload":"not base64","signatures":[{}]}`} {
		if _, err := Verify([]byte(data), nil); err == nil {
			t.Errorf("invalid envelope accepted: %s", data)
		}
	}
}

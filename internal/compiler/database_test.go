package compiler

import (
	"bytes"
	"slices"
	"testing"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/database"
)

func TestDatabaseBindingScopeAndDeterminism(t *testing.T) {
	input, selection := fixture()
	input.Definition.Resources = map[string]v1.ResourceRequest{"data": {Kind: "postgresql", Profile: "development"}}
	component := input.Definition.Components["api"]
	component.Resources = []string{"data"}
	component.Env = map[string]string{"PGHOST": "attacker.example", "PGPASSWORD": "application-override"}
	input.Definition.Components["api"] = component
	id := database.ID(selection.EnvironmentID, "data")
	a := database.Allocation{ID: id, EnvironmentID: selection.EnvironmentID, Alias: "data", Namespace: "kd-" + id[3:], Components: []string{"api"}}
	selection.Databases = map[string]v1.DatabaseBinding{"data": a.Binding()}
	first, err := Render(input, selection)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := first.Archive()
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		bundle, err := Render(input, selection)
		if err != nil {
			t.Fatal(err)
		}
		next, err := bundle.Archive()
		if err != nil || !bytes.Equal(archive, next) {
			t.Fatal("database binding changed deterministic bundle")
		}
	}
	data := first.Files["components/api/object-0.json"]
	if bytes.Contains(data, []byte("attacker.example")) || bytes.Contains(data, []byte("application-override")) || !bytes.Contains(data, []byte(`"secretKeyRef"`)) || !bytes.Contains(data, []byte(`verify-full`)) {
		t.Fatal("trusted TLS Secret binding was overridden")
	}
	if bytes.Contains(first.Files["components/web/object-0.json"], []byte("keldock-db-data")) {
		t.Fatal("unbound component received credentials")
	}
	if len(networkPolicy("api", input.Definition, selection).Spec.Egress) != 2 || len(networkPolicy("web", input.Definition, selection).Spec.Egress) != 2 {
		t.Fatal("database egress did not remain component scoped")
	}
	for _, test := range []struct {
		Name   string
		Change func(*v1.DatabaseBinding)
	}{
		{"cross-environment", func(b *v1.DatabaseBinding) { b.Namespace = "kd-other" }},
		{"other-cluster", func(b *v1.DatabaseBinding) { b.Cluster = "postgres" }},
		{"undeclared-consumer", func(b *v1.DatabaseBinding) { b.Components = append(b.Components, "web") }},
		{"arbitrary-secret", func(b *v1.DatabaseBinding) { b.Secret = "administrator" }},
	} {
		t.Run(test.Name, func(t *testing.T) {
			b := a.Binding()
			b.Components = slices.Clone(b.Components)
			test.Change(&b)
			selection.Databases["data"] = b
			if _, err := Render(input, selection); err == nil {
				t.Fatal("unsafe trusted placement accepted")
			}
		})
	}
}

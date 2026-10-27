package artifacts

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lucas-ht/keldock/internal/compiler"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

func TestOCIContentAndIdentity(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	bundle := compiler.Bundle{Files: map[string][]byte{"components/web/kustomization.json": []byte(`{"resources":[]}`)}}
	first, err := pack(ctx, store, bundle)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pack(ctx, store, bundle)
	if err != nil || first.Digest != second.Digest {
		t.Fatalf("OCI identity changed: %v", err)
	}
	data, err := content.FetchAll(ctx, store, first)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Layers) != 1 || manifest.Layers[0].MediaType != LayerMediaType {
		t.Fatal("missing extracted content layer")
	}
	layer, err := content.FetchAll(ctx, store, manifest.Layers[0])
	if err != nil || len(layer) == 0 {
		t.Fatalf("missing bundle bytes: %v", err)
	}
	name, digest, err := SplitReference("registry.example/bundles@" + first.Digest.String())
	if err != nil || name != "registry.example/bundles" || digest != first.Digest.String() {
		t.Fatalf("reference: %s %s %v", name, digest, err)
	}
	for _, reference := range []string{"registry.example/bundles:latest", "registry.example/bundles@sha256:short", "https://registry.example/bundles@" + first.Digest.String()} {
		if _, _, err := SplitReference(reference); err == nil {
			t.Errorf("invalid reference accepted: %s", reference)
		}
	}
}

func TestRegistryConfiguration(t *testing.T) {
	if _, err := NewPublisher(RegistryConfig{PushRepository: "localhost:5010/bundles", PullRepository: "registry:5000/bundles", PlainHTTP: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPublisher(RegistryConfig{PushRepository: "localhost:5010/bundles", PullRepository: "registry:5000/other"}); err == nil {
		t.Fatal("inconsistent registry alias accepted")
	}
}

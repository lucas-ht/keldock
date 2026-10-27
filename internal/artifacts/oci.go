// Package artifacts publishes compiler output independently of user image builds.
package artifacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/compiler"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// LayerMediaType identifies the single extracted deployment-content layer.
const LayerMediaType = "application/vnd.cncf.flux.content.v1.tar+gzip"

// RegistryConfig is operator-owned publication/pull routing, never application input.
// Different hosts with the same repository path support a local registry's host/cluster aliases.
type RegistryConfig struct {
	PushRepository string
	PullRepository string
	PlainHTTP      bool
	Username       string
	Password       string
}

// Publisher writes immutable artifacts to a single configured OCI repository.
type Publisher struct {
	repository *remote.Repository
	pull       string
}

// NewPublisher validates the operator-owned registry endpoint and scoped credentials.
func NewPublisher(config RegistryConfig) (*Publisher, error) {
	repository, err := remote.NewRepository(config.PushRepository)
	if err != nil {
		return nil, fmt.Errorf("bundle registry: %w", err)
	}
	pull, err := remote.NewRepository(config.PullRepository)
	if err != nil || pull.Reference.Repository != repository.Reference.Repository {
		return nil, fmt.Errorf("push/pull registry aliases must identify the same repository path")
	}
	repository.PlainHTTP = config.PlainHTTP
	repository.Client = &auth.Client{Client: &http.Client{Timeout: 30 * time.Second}, Cache: auth.NewCache(),
		Credential: auth.StaticCredential(repository.Reference.Registry, auth.Credential{Username: config.Username, Password: config.Password})}
	return &Publisher{repository: repository, pull: config.PullRepository}, nil
}

// Push publishes deterministic content and an immutable content-derived retention tag.
func (publisher *Publisher) Push(ctx context.Context, bundle compiler.Bundle) (string, error) {
	descriptor, err := pack(ctx, publisher.repository, bundle)
	if err != nil {
		return "", err
	}
	if err := publisher.repository.Tag(ctx, descriptor, "bundle-"+descriptor.Digest.Encoded()); err != nil {
		return "", fmt.Errorf("retain bundle manifest: %w", err)
	}
	return publisher.pull + "@" + descriptor.Digest.String(), nil
}

func pack(ctx context.Context, target content.Pusher, bundle compiler.Bundle) (ocispec.Descriptor, error) {
	archive, err := bundle.Archive()
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	layer := content.NewDescriptorFromBytes(LayerMediaType, archive)
	if err := target.Push(ctx, layer, bytes.NewReader(archive)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("publish bundle layer: %w", err)
	}
	layer.Annotations = map[string]string{ocispec.AnnotationTitle: "manifests.tar.gz"}
	descriptor, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1,
		"application/vnd.cncf.flux.config.v1+json", oras.PackManifestOptions{Layers: []ocispec.Descriptor{layer},
			ManifestAnnotations: map[string]string{ocispec.AnnotationCreated: "1970-01-01T00:00:00Z"}})
	if err != nil {
		return descriptor, fmt.Errorf("publish bundle manifest: %w", err)
	}
	return descriptor, nil
}

// SplitReference separates an immutable OCI artifact address for trusted Flux registration.
func SplitReference(reference string) (string, string, error) {
	name, digest, found := strings.Cut(reference, "@sha256:")
	if !found || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return "", "", fmt.Errorf("bundle requires exact sha256 digest")
	}
	if _, err := remote.NewRepository(name); err != nil {
		return "", "", fmt.Errorf("bundle repository: %w", err)
	}
	return name, "sha256:" + digest, nil
}

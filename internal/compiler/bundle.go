// Package compiler turns validated application definitions into allowlisted OCI content.
package compiler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Selection is trusted placement/configuration resolved by the control plane.
type Selection = v1.Placement

// Bundle contains deterministic manifests plus the graph the trusted registrar uses.
type Bundle struct {
	Files      map[string][]byte
	Components []string
}

// Render compiles only workload objects; namespace/RBAC/Flux ownership stays with the registrar.
func Render(input v1.SubmitRelease, selection Selection) (Bundle, error) {
	result := Bundle{Files: map[string][]byte{}}
	if err := input.Validate(); err != nil {
		return result, err
	}
	if err := validateCloud(input.Definition, selection); err != nil {
		return result, err
	}
	if err := validateDatabases(input.Definition, selection); err != nil {
		return result, err
	}
	if !v1.ValidName(selection.Namespace) || !v1.ValidName(selection.EnvironmentID) ||
		!v1.ValidName(selection.DeploymentID) || selection.Generation < 1 {
		return result, fmt.Errorf("trusted selection requires valid identities and generation")
	}
	if strings.ContainsAny(selection.Hostname, "/:@ \n") {
		return result, fmt.Errorf("invalid routed hostname")
	}
	order, err := input.Definition.Order()
	if err != nil {
		return result, err
	}
	result.Components = order
	for _, name := range order {
		component := input.Definition.Components[name]
		objects := workload(name, component, input.Images[name], selection)
		objects = append(objects, networkPolicy(name, input.Definition, selection))
		resources := make([]string, 0, len(objects))
		for index, object := range objects {
			file := fmt.Sprintf("object-%d.json", index)
			data, err := json.Marshal(object)
			if err != nil {
				return result, fmt.Errorf("encode component %s: %w", name, err)
			}
			result.Files["components/"+name+"/"+file] = data
			resources = append(resources, file)
		}
		data, err := json.Marshal(map[string]any{"apiVersion": "kustomize.config.k8s.io/v1beta1", "kind": "Kustomization", "resources": resources})
		if err != nil {
			return result, fmt.Errorf("encode kustomization: %w", err)
		}
		result.Files["components/"+name+"/kustomization.yaml"] = data
	}
	return result, nil
}

// Archive returns byte-identical tar+gzip content for identical compiler input.
func (bundle Bundle) Archive() ([]byte, error) {
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	names := make([]string, 0, len(bundle.Files))
	for name := range bundle.Files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			return nil, fmt.Errorf("unsafe bundle path %s", name)
		}
		data := bundle.Files[name]
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, fmt.Errorf("archive header: %w", err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			return nil, fmt.Errorf("archive content: %w", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return output.Bytes(), nil
}

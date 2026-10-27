package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"github.com/lucas-ht/keldock/internal/compiler"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/registryauth"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type pullPlan struct {
	bundle    []registryauth.Credential
	workloads []registryauth.Credential
}

func pullPatches(name string, component v1.Component, image string, selection compiler.Selection, plan pullPlan) ([]map[string]any, error) {
	repository, _, err := artifacts.SplitReference(image)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(plan.workloads, func(credential registryauth.Credential) bool { return credential.Repository == repository }) {
		return nil, nil
	}
	kind, path, resourceName := "Deployment", "/spec/template/spec/imagePullSecrets", name
	if component.Kind == "job" {
		kind = "Job"
		resourceName = compiler.JobName(name, selection.DeploymentID)
	}
	if component.Kind == "cronjob" {
		kind = "CronJob"
		path = "/spec/jobTemplate/spec/template/spec/imagePullSecrets"
	}
	patch, err := json.Marshal([]map[string]any{{"op": "add", "path": path, "value": []map[string]string{{"name": "keldock-image-pull"}}}})
	if err != nil {
		return nil, err
	}
	return []map[string]any{{"target": map[string]string{"kind": kind, "name": "^" + resourceName + "$"}, "patch": string(patch)}}, nil
}

func (registrar *Registrar) pullPlan(desired v1.ProjectedDeployment) (pullPlan, error) {
	config, err := registryauth.Read(registrar.CredentialsFile, registrar.PlainHTTP)
	if err != nil {
		return pullPlan{}, err
	}
	plan := pullPlan{}
	if config.Bundle != nil {
		if config.Bundle.Repository != registrar.Repository {
			return plan, fmt.Errorf("bundle pull credential outside enrolled OCI repository")
		}
		plan.bundle = []registryauth.Credential{*config.Bundle}
	}
	plan.workloads, err = config.WorkloadCredentials(desired.Operation.ProjectID, desired.Images)
	return plan, err
}

func (registrar *Registrar) projectPullSecrets(ctx context.Context, selection compiler.Selection, plan pullPlan) error {
	for name, profile := range map[string][]registryauth.Credential{"keldock-bundle-pull": plan.bundle, "keldock-image-pull": plan.workloads} {
		if len(profile) == 0 {
			continue
		}
		data, err := registryauth.DockerConfig(profile, name == "keldock-bundle-pull")
		if err != nil {
			return err
		}
		meta := metadata(name, selection)
		meta.Labels["reconcile.fluxcd.io/watch"] = "Enabled"
		secret := &corev1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: meta, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: data}}
		if err := registrar.apply(ctx, secret, selection.EnvironmentID, selection.Generation); err != nil {
			return fmt.Errorf("project registry pull secret: %w", err)
		}
	}
	return nil
}

func (registrar *Registrar) removeUnusedPullSecrets(ctx context.Context, selection compiler.Selection, plan pullPlan) error {
	for name, profile := range map[string][]registryauth.Credential{"keldock-bundle-pull": plan.bundle, "keldock-image-pull": plan.workloads} {
		if len(profile) > 0 {
			continue
		}
		if _, err := registrar.remove(ctx, "Secret", selection.Namespace, name, selection.EnvironmentID, selection.Generation); err != nil {
			return err
		}
	}
	return nil
}

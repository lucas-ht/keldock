// Package registryauth reads operator-owned pull credentials independently of releases.
package registryauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lucas-ht/keldock/internal/artifacts"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// Credential grants one exact OCI repository, not every image on a registry host.
type Credential struct {
	Repository string `json:"repository"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// Config separates bundle authentication from project-specific workload image readers.
// This document is private agent configuration and must never enter OCI bundles or APIs.
type Config struct {
	Bundle    *Credential             `json:"bundle,omitempty"`
	Workloads map[string][]Credential `json:"workloads"`
}

// Read reloads atomically replaced owner-only files so rotation needs no agent restart.
func Read(path string, development bool) (Config, error) {
	config := Config{Workloads: map[string][]Credential{}}
	if path == "" {
		return config, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return config, fmt.Errorf("pull credential file unavailable")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 1024*1024 {
		return config, fmt.Errorf("pull credential file requires bounded owner-only regular-file permissions")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("pull credential file unavailable")
	}
	if err := v1.DecodeJSON(data, &config); err != nil {
		return config, fmt.Errorf("invalid pull credential document")
	}
	if err := config.Validate(development); err != nil {
		return config, err
	}
	return config, nil
}

// Validate implements the first GHCR profile plus explicit local-development registries.
func (config Config) Validate(development bool) error {
	if len(config.Workloads) > 32 {
		return fmt.Errorf("too many project pull profiles")
	}
	validate := func(credential Credential) error {
		if _, _, err := artifacts.SplitReference(credential.Repository + "@sha256:" + strings.Repeat("0", 64)); err != nil {
			return fmt.Errorf("invalid pull repository")
		}
		if !development && !strings.HasPrefix(credential.Repository, "ghcr.io/") {
			return fmt.Errorf("only the GHCR pull profile is supported outside local development")
		}
		if credential.Repository != strings.ToLower(credential.Repository) || credential.Username == "" || len(credential.Username) > 128 || len(credential.Password) < 8 || len(credential.Password) > 4096 || strings.ContainsAny(credential.Username+credential.Password, "\r\n\x00") {
			return fmt.Errorf("invalid pull credential")
		}
		return nil
	}
	if config.Bundle != nil {
		if err := validate(*config.Bundle); err != nil {
			return err
		}
	}
	for project, credentials := range config.Workloads {
		if !v1.ValidName(project) || len(credentials) > 16 {
			return fmt.Errorf("invalid project pull profile")
		}
		seen := map[string]bool{}
		for _, credential := range credentials {
			if err := validate(credential); err != nil {
				return err
			}
			if seen[credential.Repository] {
				return fmt.Errorf("duplicate pull repository")
			}
			seen[credential.Repository] = true
		}
	}
	return nil
}

// WorkloadCredentials selects only credentials for repositories in this project's exact release.
func (config Config) WorkloadCredentials(project string, images map[string]string) ([]Credential, error) {
	repositories := map[string]bool{}
	for _, image := range images {
		repository, _, err := artifacts.SplitReference(image)
		if err != nil {
			return nil, fmt.Errorf("invalid selected workload image")
		}
		repositories[repository] = true
	}
	selected := []Credential{}
	for _, credential := range config.Workloads[project] {
		if repositories[credential.Repository] {
			selected = append(selected, credential)
		}
	}
	return selected, nil
}

// DockerConfig returns private Secret data. Flux uses host keys; kubelet supports repository keys.
func DockerConfig(credentials []Credential, hostOnly bool) ([]byte, error) {
	auths := map[string]map[string]string{}
	for _, credential := range credentials {
		key := credential.Repository
		if hostOnly {
			key = strings.SplitN(key, "/", 2)[0]
		}
		auths[key] = map[string]string{"username": credential.Username, "password": credential.Password, "auth": base64.StdEncoding.EncodeToString([]byte(credential.Username + ":" + credential.Password))}
	}
	return json.Marshal(map[string]any{"auths": auths})
}

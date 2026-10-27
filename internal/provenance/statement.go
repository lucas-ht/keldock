package provenance

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

const buildType = "https://github.com/lucas-ht/keldock/buildtypes/github-actions/v1"

// Subject is an in-toto artifact name and digest.
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Parameters bind a build's source and application definition to admitted artifacts.
type Parameters struct {
	ProjectID        string    `json:"projectId"`
	DefinitionDigest string    `json:"definitionDigest"`
	Source           v1.Source `json:"source"`
}

// Statement is Keldock's supported SLSA provenance v1 profile.
type Statement struct {
	Type          string    `json:"_type"`
	Subjects      []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     struct {
		BuildDefinition struct {
			BuildType          string     `json:"buildType"`
			ExternalParameters Parameters `json:"externalParameters"`
		} `json:"buildDefinition"`
		RunDetails struct {
			Builder struct {
				ID string `json:"id"`
			} `json:"builder"`
			Metadata struct {
				InvocationID string `json:"invocationId"`
			} `json:"metadata"`
		} `json:"runDetails"`
	} `json:"predicate"`
}

// Hash returns a canonical Go JSON SHA-256 digest for immutable contract input.
func Hash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode immutable input: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// NewStatement constructs provenance for a completed CI build with prebuilt images.
func NewStatement(input v1.SubmitRelease) (Statement, error) {
	var statement Statement
	digest, err := Hash(input.Definition)
	if err != nil {
		return statement, err
	}
	statement.Type = "https://in-toto.io/Statement/v1"
	statement.PredicateType = "https://slsa.dev/provenance/v1"
	statement.Predicate.BuildDefinition.BuildType = buildType
	statement.Predicate.BuildDefinition.ExternalParameters = Parameters{input.ProjectID, digest, input.Source}
	statement.Predicate.RunDetails.Builder.ID = "https://github.com/" + input.Source.WorkflowRef
	statement.Predicate.RunDetails.Metadata.InvocationID = fmt.Sprintf("https://github.com/%s/actions/runs/%d/attempts/%d", input.Source.Repository, input.Source.RunID, input.Source.RunAttempt)
	names, err := input.Definition.Order()
	if err != nil {
		return statement, err
	}
	for _, component := range names {
		image := strings.Split(input.Images[component], "@sha256:")
		if len(image) != 2 {
			return statement, fmt.Errorf("component %s requires a sha256 image digest", component)
		}
		statement.Subjects = append(statement.Subjects, Subject{image[0], map[string]string{"sha256": image[1]}})
	}
	return statement, nil
}

// VerifyRelease requires signed subjects, source, build definition and invocation parity.
func VerifyRelease(input v1.SubmitRelease, keys map[string]ed25519.PublicKey) error {
	body, err := Verify(input.Provenance, keys)
	if err != nil {
		return err
	}
	var statement Statement
	if err := v1.DecodeJSON(body, &statement); err != nil {
		return fmt.Errorf("provenance profile: %w", err)
	}
	expected, err := NewStatement(input)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(statement, expected) {
		return fmt.Errorf("signed provenance does not match complete release, source and workflow")
	}
	return nil
}

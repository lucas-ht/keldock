package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// prepareCommand assembles trusted CI inputs; signing remains an explicit separate step.
func prepareCommand(args []string) error {
	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	definitionPath := flags.String("file", "keldock.yaml", "application definition")
	imagesPath := flags.String("images", "", "complete component-to-digest JSON")
	project := flags.String("project", "", "operator-configured project")
	output := flags.String("output", "", "new unsigned release JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *imagesPath == "" || *output == "" || !v1.ValidName(*project) {
		return fmt.Errorf("images, project and output are required")
	}
	data, err := boundedFile(*definitionPath, 1024*1024)
	if err != nil {
		return err
	}
	definition, err := v1.DecodeApplication(data)
	if err != nil {
		return err
	}
	data, err = boundedFile(*imagesPath, 65536)
	if err != nil {
		return err
	}
	var images map[string]string
	if err := v1.DecodeJSON(data, &images); err != nil {
		return err
	}
	source, err := githubSource()
	if err != nil {
		return err
	}
	input := v1.SubmitRelease{APIVersion: v1.APIVersion, ProjectID: *project, Definition: definition, Images: images, Source: source, Provenance: json.RawMessage(`{}`)}
	if err := input.Validate(); err != nil {
		return err
	}
	data, err = json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNew(*output, append(data, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Println("Unsigned complete release prepared; sign it in trusted CI before submission")
	return nil
}

func boundedFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fmt.Errorf("input must be a bounded regular file")
	}
	return os.ReadFile(path)
}

func githubSource() (v1.Source, error) {
	var source v1.Source
	positive := func(name string) (int64, error) {
		value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("required positive GitHub variable %s is missing", name)
		}
		return value, nil
	}
	var err error
	if source.RepositoryID, err = positive("GITHUB_REPOSITORY_ID"); err != nil {
		return source, err
	}
	if source.OwnerID, err = positive("GITHUB_REPOSITORY_OWNER_ID"); err != nil {
		return source, err
	}
	if source.RunID, err = positive("GITHUB_RUN_ID"); err != nil {
		return source, err
	}
	if source.RunAttempt, err = positive("GITHUB_RUN_ATTEMPT"); err != nil {
		return source, err
	}
	source.Repository = os.Getenv("GITHUB_REPOSITORY")
	source.Commit = os.Getenv("GITHUB_SHA")
	source.Ref = os.Getenv("GITHUB_REF")
	source.Event = os.Getenv("GITHUB_EVENT_NAME")
	source.WorkflowRef = os.Getenv("GITHUB_WORKFLOW_REF")
	if source.Event == "pull_request" {
		data, err := boundedFile(os.Getenv("GITHUB_EVENT_PATH"), 1024*1024)
		if err != nil {
			return source, err
		}
		var event struct {
			Number      int64 `json:"number"`
			PullRequest struct {
				Head struct {
					SHA  string `json:"sha"`
					Repo struct {
						ID int64 `json:"id"`
					} `json:"repo"`
				} `json:"head"`
			} `json:"pull_request"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			return source, fmt.Errorf("invalid pull-request event")
		}
		if event.Number <= 0 || event.PullRequest.Head.Repo.ID != source.RepositoryID {
			return source, fmt.Errorf("fork or unknown pull requests cannot access the signing/deployment profile")
		}
		source.PullRequest = event.Number
		source.HeadCommit = event.PullRequest.Head.SHA
	}
	if source.Event != "push" && source.Event != "pull_request" && source.Event != "workflow_dispatch" {
		return source, fmt.Errorf("unsupported GitHub CI event")
	}
	return source, nil
}

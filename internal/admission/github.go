// Package admission binds authenticated CI identity, repository policy and signed artifact evidence.
package admission

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
)

// ErrDenied never includes bearer credentials or untrusted provenance in its text.
var ErrDenied = errors.New("CI identity, target or provenance denied")

// RepositoryPolicy is operator-owned trust, independent of application configuration.
type RepositoryPolicy struct {
	v1.Scope
	RepositoryID int64
	OwnerID      int64
	WorkflowPath string
	PushRef      string
	Keys         map[string]ed25519.PublicKey
}

// Principal contains only source identity established by a verified CI token.
type Principal struct {
	v1.Scope
	Actor         string
	Source        v1.Source
	authenticated bool
}

// GitHub verifies Actions OIDC with an instance-specific audience and an approved direct workflow.
type GitHub struct {
	verifier *oidc.IDTokenVerifier
	policies map[int64]RepositoryPolicy
}

// NewGitHub allows GitHub.com, or an explicitly selected loopback issuer for tests.
// Repository callers cannot supply issuers, public keys or authorization scope.
func NewGitHub(ctx context.Context, issuer, audience string, policies []RepositoryPolicy, allowLocalIssuer bool) (*GitHub, error) {
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || audience == "" {
		return nil, fmt.Errorf("invalid CI verifier configuration")
	}
	local := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if issuer != "https://token.actions.githubusercontent.com" && (!allowLocalIssuer || !local || parsed.Scheme != "http") {
		return nil, fmt.Errorf("unsupported Actions issuer")
	}
	trusted := map[int64]RepositoryPolicy{}
	for _, policy := range policies {
		if !v1.ValidName(policy.OrganizationID) || !v1.ValidName(policy.ProjectID) || policy.RepositoryID <= 0 || policy.OwnerID <= 0 || !strings.HasPrefix(policy.WorkflowPath, ".github/workflows/") || strings.Contains(policy.WorkflowPath, "..") || !strings.HasPrefix(policy.PushRef, "refs/heads/") || len(policy.Keys) == 0 {
			return nil, fmt.Errorf("invalid repository CI policy")
		}
		if _, exists := trusted[policy.RepositoryID]; exists {
			return nil, fmt.Errorf("repository may belong to only one project per installation")
		}
		keys := map[string]ed25519.PublicKey{}
		for id, key := range policy.Keys {
			if len(key) != ed25519.PublicKeySize || id == "" {
				return nil, fmt.Errorf("invalid provenance public key")
			}
			keys[id] = append(ed25519.PublicKey(nil), key...)
		}
		policy.Keys = keys
		trusted[policy.RepositoryID] = policy
	}
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second})
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover CI issuer: %w", err)
	}
	return &GitHub{verifier: provider.Verifier(&oidc.Config{ClientID: audience, SupportedSigningAlgs: []string{oidc.RS256}}), policies: trusted}, nil
}

type claims struct {
	RepositoryID   string `json:"repository_id"`
	OwnerID        string `json:"repository_owner_id"`
	Repository     string `json:"repository"`
	Commit         string `json:"sha"`
	Ref            string `json:"ref"`
	Event          string `json:"event_name"`
	WorkflowRef    string `json:"workflow_ref"`
	JobWorkflowRef string `json:"job_workflow_ref"`
	RunID          string `json:"run_id"`
	RunAttempt     string `json:"run_attempt"`
}

var pullRef = regexp.MustCompile(`^refs/pull/([1-9][0-9]*)/merge$`)

// Authenticate establishes server-owned scope for submission and subsequent status reads.
func (github *GitHub) Authenticate(ctx context.Context, bearer string) (Principal, error) {
	var principal Principal
	token, err := github.verifier.Verify(ctx, bearer)
	if err != nil || token.Subject == "" {
		return principal, ErrDenied
	}
	var identity claims
	if err := token.Claims(&identity); err != nil {
		return principal, ErrDenied
	}
	repository, err := strconv.ParseInt(identity.RepositoryID, 10, 64)
	if err != nil {
		return principal, ErrDenied
	}
	policy, ok := github.policies[repository]
	if !ok || strconv.FormatInt(policy.OwnerID, 10) != identity.OwnerID ||
		(identity.JobWorkflowRef != "" && identity.JobWorkflowRef != identity.WorkflowRef) ||
		identity.WorkflowRef != identity.Repository+"/"+policy.WorkflowPath+"@"+identity.Ref {
		return principal, ErrDenied
	}
	run, err := strconv.ParseInt(identity.RunID, 10, 64)
	if err != nil || run < 1 {
		return principal, ErrDenied
	}
	attempt, err := strconv.ParseInt(identity.RunAttempt, 10, 64)
	if err != nil || attempt < 1 {
		return principal, ErrDenied
	}
	var number int64
	if identity.Event == "pull_request" {
		match := pullRef.FindStringSubmatch(identity.Ref)
		if len(match) != 2 {
			return principal, ErrDenied
		}
		number, err = strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return principal, ErrDenied
		}
	} else if (identity.Event != "push" && identity.Event != "workflow_dispatch") || identity.Ref != policy.PushRef {
		return principal, ErrDenied
	}
	principal = Principal{Scope: policy.Scope, authenticated: true, Actor: fmt.Sprintf("github:%d:run:%d:attempt:%d", repository, run, attempt), Source: v1.Source{
		RepositoryID: repository, OwnerID: policy.OwnerID, Repository: identity.Repository, Commit: identity.Commit, Ref: identity.Ref, Event: identity.Event, WorkflowRef: identity.WorkflowRef, RunID: run, RunAttempt: attempt, PullRequest: number}}
	return principal, nil
}

// VerifyRelease checks source parity and signed artifacts after structural validation.
// Pull-request head identity is also checked against authenticated webhook state by storage.
func (github *GitHub) VerifyRelease(principal Principal, input v1.SubmitRelease) error {
	if err := input.Validate(); err != nil {
		return err
	}
	policy, ok := github.policies[principal.Source.RepositoryID]
	if !principal.authenticated || !ok || principal.Scope != policy.Scope || input.ProjectID != policy.ProjectID {
		return ErrDenied
	}
	source := input.Source
	source.HeadCommit = ""
	if !reflect.DeepEqual(source, principal.Source) {
		return ErrDenied
	}
	if err := provenance.VerifyRelease(input, policy.Keys); err != nil {
		return ErrDenied
	}
	return nil
}

package controlplane

import (
	"net/http"
	"strings"

	"github.com/lucas-ht/keldock/internal/access"
)

type setupStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Detail string `json:"detail"`
	Href   string `json:"href,omitempty"`
}

// onboarding reports observed setup state; it never treats a checklist as authority.
func (api *API) onboarding(w http.ResponseWriter, r *http.Request) {
	p, err := api.Security.Human(r)
	if err != nil || !p.IsAdmin() {
		failure(w, access.ErrDenied)
		return
	}
	agent, err := api.Security.Clusters.Information(r.Context())
	if err != nil {
		failure(w, err)
		return
	}
	transport := "development"
	if strings.HasPrefix(api.Security.MCP.Issuer, "https://") {
		transport = "configured"
	}
	steps := []setupStep{
		{"administrator", "Local administrator", "active", "Signed in with the installation’s local account. Keep the offline administrator recovery procedure with your backup.", "/account"},
		{"https", "Canonical HTTPS origin", transport, api.Security.MCP.Issuer + " · verify DNS and the certificate from the operator and agent networks with installation preflight.", ""},
		{"registrar", "Workload registrar", agent.State, "Independent enrollment controls workload reconciliation. Running applications retain their state during an outage.", "/clusters"},
	}
	githubState := "incomplete"
	if api.GitHubApp != nil {
		status, err := api.GitHubApp.Status(r.Context())
		if err != nil {
			failure(w, err)
			return
		}
		if status.App.Enabled && status.App.SyncedAt != nil && status.App.SyncError == "" {
			for _, repository := range status.Repositories {
				if repository.Active {
					githubState = "configured"
					break
				}
			}
		}
	}
	steps = append(steps, setupStep{"github", "GitHub delivery", githubState, "Connect your own App, select repositories, and confirm permissions and delivery events. CI builds and publishes immutable images.", "/settings/github"})
	projectState := "incomplete"
	if len(api.Projects) > 0 {
		projectState = "configured"
	}
	steps = append(steps, setupStep{"projects", "Repository policy", projectState, "Add a reviewed stable repository ID, workflow/ref and public provenance key using installation project. Restart after applying the private configuration.", "/projects"})
	if api.Databases != nil && len(api.Databases.Policies) > 0 {
		databaseAgent, err := api.Security.Database.Information(r.Context())
		if err != nil {
			failure(w, err)
			return
		}
		steps = append(steps, setupStep{"postgresql", "Optional PostgreSQL", databaseAgent.State, "Approved allocation profiles are configured. Review allocations and enroll the separate database provisioner from a project’s Resources page.", "/projects"})
	}
	var ready bool
	if err := api.Store.Pool().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM operations o JOIN environments e ON e.id=o.environment_id AND e.generation=o.generation JOIN observations v ON v.environment_id=e.id AND v.generation=e.generation WHERE e.organization_id=$1 AND NOT e.deleted AND o.state='ready' AND (v.report->>'observedAt')::timestamptz>now()-interval '90 seconds')`, api.Organization).Scan(&ready); err != nil {
		failure(w, err)
		return
	}
	deliveryState := "incomplete"
	if ready {
		deliveryState = "ready"
	}
	steps = append(steps,
		setupStep{"delivery", "First healthy environment", deliveryState, "Readiness requires a fresh observation for the selected generation. A successful CI submission alone is insufficient.", "/"},
		setupStep{"mcp", "MCP and agent skills", "optional", "Connect an MCP client with explicit project consent. Install the bundled preview and diagnose skills. Resource inspection never grants database administration.", "/settings/mcp"},
	)
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "projectCount": len(api.Projects), "clusterId": api.Cluster})
}

// Package agent pulls approved intent through an outbound authenticated API connection.
package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lucas-ht/keldock/internal/buildinfo"
	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// APIError identifies a bounded remote failure without exposing credentials or response bodies.
type APIError struct{ Status int }

func (err APIError) Error() string { return fmt.Sprintf("control-plane response %d", err.Status) }

// Client speaks the versioned agent API. Credentials never enter workload definitions.
type Client struct {
	mu          sync.Mutex
	session     string
	credentials *credentialState
	base        string
	token       string
	http        *http.Client
	cloud       bool
	database    bool
}

// NewClient requires HTTPS, with an explicit loopback HTTP exception for development.
func NewClient(endpoint, token string, allowLocalHTTP bool) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" || len(token) < 32 {
		return nil, fmt.Errorf("invalid agent connection")
	}
	local := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if parsed.Scheme != "https" && (!allowLocalHTTP || !local || parsed.Scheme != "http") {
		return nil, fmt.Errorf("agent API requires HTTPS")
	}
	return &Client{session: rand.Text(), base: strings.TrimSuffix(endpoint, "/"), token: token, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (client *Client) request(ctx context.Context, method, path string, input, output any) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requestLocked(ctx, method, path, input, output)
}

func (client *Client) requestLocked(ctx context.Context, method, path string, input, output any) error {
	if client.cloud && strings.HasPrefix(path, "/v1/agent/") {
		path = "/v1/cloud/agent/" + strings.TrimPrefix(path, "/v1/agent/")
	}
	if client.database && strings.HasPrefix(path, "/v1/agent/") {
		path = "/v1/database/agent/" + strings.TrimPrefix(path, "/v1/agent/")
	}
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, client.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Keldock-Agent-Session", client.session)
	request.Header.Set("X-Keldock-Agent-Version", buildinfo.Version)
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("control-plane connection unavailable")
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			slog.Debug("agent response close interrupted", "error", err)
		}
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return APIError{Status: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		return fmt.Errorf("invalid or oversized control-plane response")
	}
	if output != nil {
		return v1.DecodeJSON(data, output)
	}
	return nil
}

// Desired retrieves a complete bounded-page snapshot for the authenticated cluster.
func (client *Client) Desired(ctx context.Context) ([]v1.ProjectedDeployment, error) {
	var result []v1.ProjectedDeployment
	cursor := ""
	for pages := 0; pages < 200; pages++ {
		var page struct {
			APIVersion string                   `json:"apiVersion"`
			Items      []v1.ProjectedDeployment `json:"items"`
			Next       string                   `json:"nextCursor"`
		}
		if err := client.request(ctx, http.MethodGet, "/v1/agent/desired?after="+url.QueryEscape(cursor), nil, &page); err != nil {
			return nil, err
		}
		if page.APIVersion != v1.APIVersion || len(page.Items) > 5 || (page.Next != "" && page.Next <= cursor) {
			return nil, fmt.Errorf("unsupported or invalid agent protocol page")
		}
		result = append(result, page.Items...)
		if page.Next == "" {
			return result, nil
		}
		cursor = page.Next
	}
	return nil, fmt.Errorf("agent environment limit exceeded")
}

// Current revalidates cached selection immediately before cluster mutation.
func (client *Client) Current(ctx context.Context, environment string) (*v1.ProjectedDeployment, error) {
	if !v1.ValidName(environment) {
		return nil, fmt.Errorf("invalid environment identity")
	}
	var desired v1.ProjectedDeployment
	if err := client.request(ctx, http.MethodGet, "/v1/agent/environments/"+environment, nil, &desired); err != nil {
		return nil, err
	}
	return &desired, nil
}

// Observe reports revision-aware evidence independently of the original submitting client.
func (client *Client) Observe(ctx context.Context, report v1.Observation) error {
	return client.request(ctx, http.MethodPost, "/v1/agent/observations", report, nil)
}

// SessionID identifies this process lease, without granting access by itself.
func (client *Client) SessionID() string { return client.session }

// Heartbeat maintains the exclusive lease while a long cluster operation is running.
func (client *Client) Heartbeat(ctx context.Context) error {
	return client.request(ctx, http.MethodPost, "/v1/agent/heartbeat", nil, nil)
}

// Release allows a replacement process to reconnect immediately after graceful shutdown.
func (client *Client) Release(ctx context.Context) error {
	return client.request(ctx, http.MethodPost, "/v1/agent/session/release", nil, nil)
}

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
)

// Error classifies retryable GitHub failures without returning response bodies or tokens.
type Error struct {
	Status     int
	RetryAfter time.Duration
}

func (failure *Error) Error() string {
	return fmt.Sprintf("GitHub request failed (%d)", failure.Status)
}

// Options allows a loopback API only in explicit development tests.
type Options struct {
	BaseURL     string
	Development bool
	Transport   http.RoundTripper
}

// Client owns a private credential snapshot. Rotation constructs a fresh client.
type Client struct {
	credentials credentials.App
	base        string
	http        *http.Client
}

// New validates credentials and the API origin without performing network requests.
func New(app credentials.App, options Options) (*Client, error) {
	if _, err := app.Key(); err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(options.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return nil, fmt.Errorf("invalid GitHub API origin")
	}
	local := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	localAPI := options.Development && local && parsed.Scheme == "http"
	if base != "https://api.github.com" && !localAPI {
		return nil, fmt.Errorf("unsupported GitHub API origin")
	}
	return &Client{credentials: app, base: base, http: &http.Client{Timeout: 15 * time.Second, Transport: options.Transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (client *Client) request(ctx context.Context, method, path, token string, input, output any) error {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "://") {
		return fmt.Errorf("invalid GitHub API path")
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
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", APIVersion)
	request.Header.Set("User-Agent", "Keldock/v1alpha1")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("GitHub transport unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retry := 5 * time.Second
		if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
			retry = min(time.Duration(seconds)*time.Second, time.Hour)
		} else if response.Header.Get("X-RateLimit-Remaining") == "0" {
			if reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				retry = min(max(time.Until(time.Unix(reset, 0)), time.Second), time.Hour)
			}
		}
		return &Error{Status: response.StatusCode, RetryAfter: retry}
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return fmt.Errorf("GitHub response exceeded the supported bound")
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("invalid GitHub response")
	}
	return nil
}

package admission

import (
	"context"
	"testing"
)

func TestGitHubInitializationDoesNotRequireIssuerConnectivity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewGitHub(ctx, "https://token.actions.githubusercontent.com", "https://installation.example", nil, false); err != nil {
		t.Fatalf("issuer discovery blocked control-plane startup: %v", err)
	}
}

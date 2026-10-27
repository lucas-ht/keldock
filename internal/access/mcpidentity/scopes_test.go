package mcpidentity_test

import (
	"testing"

	"github.com/lucas-ht/keldock/internal/access/mcpidentity"
)

func TestExplicitScopeNormalization(t *testing.T) {
	for _, test := range []struct {
		input, want string
		valid       bool
	}{
		{"", mcpidentity.ReadScope, true},
		{mcpidentity.ReadScope, mcpidentity.ReadScope, true},
		{"keldock:operate keldock:read", mcpidentity.OperationScope, true},
		{mcpidentity.OperateScope, "", false},
		{"keldock:read keldock:admin", "", false},
		{"keldock:read keldock:read", "", false},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := mcpidentity.NormalizeScope(test.input)
			if (err == nil) != test.valid || got != test.want {
				t.Fatalf("scope %q: %q, %v", test.input, got, err)
			}
		})
	}
}

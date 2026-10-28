package registry

import (
	"context"
	"testing"
)

func TestGovernanceFailsClosedBeforeDatabase(t *testing.T) {
	cases := []struct{ name, enabled, demo, address, dsn string }{
		{"missing demo opt in", "true", "", "127.0.0.1:12121", "postgres://localhost:5432/test_governance_demo"},
		{"missing auth configuration", "true", "true", "127.0.0.1:12121", "postgres://localhost:5432/test_governance_demo"},
		{"untrusted binding", "true", "true", ":12121", "postgres://localhost:5432/test_governance_demo"},
		{"non demo database", "true", "true", "127.0.0.1:12121", "postgres://localhost:5432/production"},
		{"remote database", "true", "true", "127.0.0.1:12121", "postgres://example.com:5432/test_governance_demo"},
		{"invalid mode flag", "tru", "true", "127.0.0.1:12121", "postgres://localhost:5432/test_governance_demo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AGENT_REGISTRY_GOVERNANCE_ENABLED", c.enabled)
			t.Setenv("AGENT_REGISTRY_GOVERNANCE_LOCAL_DEMO", c.demo)
			t.Setenv("AGENT_REGISTRY_GOVERNANCE_AUTH_FILE", "")
			t.Setenv("AGENT_REGISTRY_SERVER_ADDRESS", c.address)
			t.Setenv("AGENT_REGISTRY_DATABASE_URL", c.dsn)
			if e := App(context.Background()); e == nil {
				t.Fatal("unsafe startup accepted")
			}
		})
	}
}

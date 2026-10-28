package governance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"golang.org/x/crypto/bcrypt"
)

func TestValidate(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name, kind string
		p          Payload
		ok         bool
	}{
		{"agent", "agent", Payload{Name: "ktv-agent", Purpose: "support", Description: "business", Risk: "low"}, true},
		{"empty purpose", "agent", Payload{Name: "a", Description: "business", Risk: "low"}, false},
		{"instance", "instance", Payload{Name: "dev", Environment: "dev", Location: "workstation"}, true},
		{"bad environment", "instance", Payload{Name: "dev", Environment: "uat", Location: "workstation"}, false},
		{"credential endpoint", "instance", Payload{Name: "dev", Environment: "dev", Location: "workstation", Endpoint: "https://u:pass@example.com"}, false},
		{"tool underscore", "tool", Payload{Name: "create_issue", Description: "write", Risk: "medium", Classification: "write", Snapshot: "v1", InputSchema: []byte(`{"type":"object"}`)}, true},
		{"invalid schema", "tool", Payload{Name: "a", Description: "x", Risk: "low", Classification: "read", Snapshot: "v1", InputSchema: []byte(`{"type":"nonsense"}`)}, false},
		{"remote schema", "tool", Payload{Name: "a", Description: "x", Risk: "low", Classification: "read", Snapshot: "v1", InputSchema: []byte(`{"$ref":"https://example.com/schema"}`)}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e := Validate(tt.kind, tt.p, now)
			if (e == nil) != tt.ok {
				t.Fatalf("got %v", e)
			}
		})
	}
}
func TestExpiryAndCleaning(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Second)
	r := Record{Kind: "grant", State: "approved", Data: Payload{ExpiresAt: &past}}
	if Effective(r, now).State != "expired" {
		t.Fatal("server expiry")
	}
	r.State = "revoked"
	if Effective(r, now).State != "revoked" {
		t.Fatal("revoked must stay revoked")
	}
	p := CleanPayload("agent", Payload{Name: "a", RequestType: "access", Constraints: map[string]string{"admin": "true"}})
	if p.RequestType != "" || p.Constraints != nil {
		t.Fatal("mass assignment")
	}
}

type failRepo struct {
	err   error
	calls int
}

func (r *failRepo) Within(_ context.Context, _ Actor, _ func(Unit) error) error {
	r.calls++
	return r.err
}
func (r *failRepo) List(context.Context, Actor, Filter, time.Time) (Page, error) { return Page{}, nil }
func (r *failRepo) Events(context.Context, Actor, Filter) (Events, error)        { return Events{}, nil }
func (r *failRepo) Denied(context.Context, Event) error                          { return nil }
func TestServiceRequiresSession(t *testing.T) {
	r := &failRepo{}
	s := NewService(r)
	_, e := s.Execute(context.Background(), Command{}, "")
	if !errors.Is(e, auth.ErrUnauthenticated) || r.calls != 0 {
		t.Fatal(e)
	}
}
func TestLocalAuth(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo-password"), 12)
	cfg := AuthConfig{SigningKey: RandomKey()}
	for _, id := range []string{"owner", "approver", "auditor"} {
		cfg.Accounts = append(cfg.Accounts, Account{Actor: Actor{ID: id, Name: id, Department: "ktud", Roles: []string{id}}, PasswordHash: string(hash)})
	}
	a, e := NewLocalAuth(cfg, []string{"http://localhost:3000"})
	if e != nil {
		t.Fatal(e)
	}
	token, e := a.Login("owner", "demo-password", "test")
	if e != nil {
		t.Fatal(e)
	}
	actor, e := a.Resolve(token)
	if e != nil || actor.ID != "owner" {
		t.Fatal(e)
	}
	if _, e = a.Resolve(token + "x"); e == nil {
		t.Fatal("tampering accepted")
	}
	a.now = func() time.Time { return time.Now().Add(3 * time.Hour) }
	if _, e = a.Resolve(token); e == nil {
		t.Fatal("expired session accepted")
	}
	t.Setenv("AGENT_REGISTRY_GOVERNANCE_LOCAL_DEMO", "")
	if _, e = LoadLocalAuth("127.0.0.1:12121"); e == nil {
		t.Fatal("missing auth must fail")
	}
	t.Setenv("AGENT_REGISTRY_GOVERNANCE_LOCAL_DEMO", "true")
	if _, e = LoadLocalAuth(":12121"); e == nil {
		t.Fatal("nonloopback must fail")
	}
}

package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	transport "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/governance"
	db "github.com/agentregistry-dev/agentregistry/internal/registry/database/governance"
	g "github.com/agentregistry-dev/agentregistry/internal/registry/governance"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"golang.org/x/crypto/bcrypt"
)

func TestHTTPWorkflowAndIsolation(t *testing.T) {
	pool, _ := database(t)
	repo := db.New(pool)
	hash, _ := bcrypt.GenerateFromPassword([]byte("integration-test-password"), 12)
	cfg := g.AuthConfig{SigningKey: g.RandomKey()}
	for _, a := range []g.Actor{
		{ID: "owner", Name: "Owner", Department: "ktud", Roles: []string{"owner", "approver"}},
		{ID: "approver", Name: "Approver", Department: "ktud", Roles: []string{"approver"}},
		{ID: "tools", Name: "Tool Owner", Department: "ktud", Roles: []string{"tool_owner"}},
		{ID: "auditor", Name: "Auditor", Department: "ktud", Roles: []string{"auditor"}},
		{ID: "admin", Name: "Admin", Department: "ktud", Roles: []string{"admin"}},
		{ID: "ea", Name: "EA", Department: "ea", Roles: []string{"owner", "approver"}},
	} {
		cfg.Accounts = append(cfg.Accounts, g.Account{Actor: a, PasswordHash: string(hash)})
	}
	local, e := g.NewLocalAuth(cfg, []string{"http://localhost:3000"})
	if e != nil {
		t.Fatal(e)
	}
	if e = local.Attach(context.Background(), repo); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Governance test", "1"))
	transport.Register(api, g.NewService(repo), local)
	// Stand-in upstream routes return success unless the actual guard blocks them.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	server := httptest.NewServer(local.Guard(mux))
	defer server.Close()
	cookies := map[string]*http.Cookie{}
	call := func(id, method, path string, body any, status int) []byte {
		t.Helper()
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		req, e := http.NewRequest(method, server.URL+path, bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:3000")
		if c := cookies[id]; c != nil {
			req.AddCookie(c)
		}
		res, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != status {
			t.Fatalf("%s %s as %s: got %d expected %d: %s", method, path, id, res.StatusCode, status, data)
		}
		for _, c := range res.Cookies() {
			cookies[id] = c
		}
		return data
	}
	for _, id := range []string{"owner", "approver", "tools", "auditor", "admin", "ea"} {
		call(id, "POST", "/api/governance/login", map[string]string{"id": id, "password": "integration-test-password"}, 200)
		if cookies[id] == nil || !cookies[id].HttpOnly || cookies[id].SameSite != http.SameSiteStrictMode {
			t.Fatal("protected session cookie missing")
		}
	}
	call("", "GET", "/api/governance/records", nil, 401)
	call("owner", "POST", "/api/governance/login", map[string]string{"id": "owner", "password": "wrong"}, 401)
	command := func(id string, c g.Command, status int) g.Record {
		t.Helper()
		b := call(id, "POST", "/api/governance/commands", c, status)
		var r g.Record
		if status == 200 {
			if e := json.Unmarshal(b, &r); e != nil {
				t.Fatal(e)
			}
		}
		return r
	}
	create := func(id, kind, parent, tool string, p g.Payload) g.Record {
		return command(id, g.Command{Action: "create", Kind: kind, Parent: parent, Tool: tool, Data: p}, 200)
	}
	action := func(id string, r g.Record, act, reason string, status int) g.Record {
		return command(id, g.Command{Action: act, Kind: r.Kind, ID: r.ID, BaseVersion: r.Version, Reason: reason}, status)
	}
	a := create("owner", "agent", "", "", g.Payload{Name: "http-agent", Description: "business", Purpose: "http test", Risk: "low"})
	inst := create("owner", "instance", a.ID, "", g.Payload{Name: "dev", Environment: "dev", Location: "platform"})
	review := create("owner", "request", a.ID, "", g.Payload{RequestType: "agent", Purpose: "profile review"})
	review = action("owner", review, "submit", "", 200)
	action("owner", review, "approve", "", 403)
	action("approver", review, "approve", "", 200)
	serverRecord := create("tools", "server", "", "", g.Payload{Name: "jira-mcp", Description: "metadata only"})
	tool := create("tools", "tool", serverRecord.ID, "", g.Payload{Name: "create_issue", Description: "create", Classification: "write", Risk: "medium", Snapshot: "v1", InputSchema: []byte(`{"type":"object"}`)})
	tool = action("approver", tool, "approve", "", 200)
	until := time.Now().Add(time.Hour)
	req := create("owner", "request", inst.ID, tool.ID, g.Payload{RequestType: "access", Purpose: "project demo", Operations: []string{"create_issue"}, Constraints: map[string]string{"project": "DEMO"}, ExpiresAt: &until})
	req = action("owner", req, "submit", "", 200)
	action("auditor", req, "approve", "", 403)
	action("ea", req, "approve", "", 404)
	action("approver", req, "approve", "", 200)
	action("approver", req, "approve", "", 409)
	var grants g.Page
	json.Unmarshal(call("auditor", "GET", "/api/governance/records?kind=grant&parent="+inst.ID, nil, 200), &grants)
	if grants.Total != 1 || len(grants.Items) != 1 {
		t.Fatal("grant missing")
	}
	grant := grants.Items[0]
	action("auditor", grant, "revoke", "no", 403)
	action("approver", grant, "revoke", "", 422)
	revoked := action("approver", grant, "revoke", "=formula", 200)
	if revoked.RevokedAt == nil || revoked.DecidedAt == nil || revoked.RevokedBy != "approver" {
		t.Fatal("revocation chronology lost")
	}
	// CSV uses identical scope and escapes spreadsheet formulas.
	csv := string(call("auditor", "GET", "/api/governance/audit.csv", nil, 200))
	if !strings.Contains(csv, "'=formula") {
		t.Fatalf("CSV formula unescaped: %s", csv)
	}
	var events g.Events
	json.Unmarshal(call("ea", "GET", "/api/governance/audit?limit=1", nil, 200), &events)
	if events.Total != 2 {
		t.Fatalf("EA should see only its provisioning and denied attempt: %d", events.Total)
	}
	var page g.Page
	json.Unmarshal(call("ea", "GET", "/api/governance/records?search=http&limit=1", nil, 200), &page)
	if page.Total != 0 {
		t.Fatal("cross-scope count leaked")
	}
	call("ea", "GET", "/api/governance/records/"+a.ID, nil, 404)
	for _, path := range []string{"/v0/agents", "/v0/apply", "/v0.1/servers", "/admin/test", "/mcp", "/plugin-marketplace/marketplace.json", "/v0/deployments", "/api/other"} {
		call("owner", "GET", path, nil, 403)
	}
	// Unknown ownership/role fields are rejected by the Huma schema, not trusted.
	call("owner", "POST", "/api/governance/commands", map[string]any{"action": "create", "kind": "agent", "baseVersion": 0, "owner": "admin", "department": "ea", "data": map[string]string{"name": "bad"}}, 422)
	// A browser from another origin cannot reuse a valid session cookie.
	csrf, _ := http.NewRequest("POST", server.URL+"/api/governance/logout", nil)
	csrf.AddCookie(cookies["owner"])
	csrf.Header.Set("Origin", "https://untrusted.example")
	res, e := server.Client().Do(csrf)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("csrf accepted")
	}
	// An administrator assigns roles only in its department, with base version.
	call("owner", "GET", "/api/governance/accounts", nil, 403)
	call("admin", "POST", "/api/governance/accounts/auditor/roles", map[string]any{"roles": []string{"auditor"}, "baseVersion": 1}, 200)
	call("auditor", "GET", "/api/governance/me", nil, 401) // old session invalidated
	call("admin", "POST", "/api/governance/accounts/auditor/roles", map[string]any{"roles": []string{"owner"}, "baseVersion": 1}, 409)
	call("admin", "POST", "/api/governance/accounts/ea/roles", map[string]any{"roles": []string{"owner"}, "baseVersion": 1}, 404)
	call("admin", "POST", "/api/governance/accounts/admin/roles", map[string]any{"roles": []string{"owner"}, "baseVersion": 1}, 403)
	// Reprovisioning does not reset an account's role revision or duplicate audit.
	before, _ := repo.Account(context.Background(), "auditor")
	if e = local.Attach(context.Background(), repo); e != nil {
		t.Fatal(e)
	}
	after, _ := repo.Account(context.Background(), "auditor")
	if before.Revision != after.Revision {
		t.Fatal("provision overwrote account")
	}
	fmt.Fprintln(io.Discard, "HTTP workflow uses real PostgreSQL transactions")
}

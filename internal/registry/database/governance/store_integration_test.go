package governance_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	db "github.com/agentregistry-dev/agentregistry/internal/registry/database/governance"
	g "github.com/agentregistry-dev/agentregistry/internal/registry/governance"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/v1alpha1store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func database(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("GOVERNANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GOVERNANCE_TEST_DATABASE_URL to a local trusted admin PostgreSQL URL; a disposable database is created")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	name := "governance_test_" + uuid.NewString()[:8]
	if _, e = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	u.Path = "/" + name
	testDSN := u.String()
	t.Cleanup(func() {
		admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		admin.Close(ctx)
	})
	mg, e := v1alpha1store.NewOSSMigrator(ctx, testDSN)
	if e != nil {
		t.Fatal(e)
	}
	if e = mg.Up(); e != nil {
		t.Fatal(e)
	}
	mg.Close()
	pool, e := pgxpool.New(ctx, testDSN)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	return pool, testDSN
}
func actor(id string, roles ...string) context.Context {
	return auth.AuthSessionTo(context.Background(), g.Actor{ID: id, Name: id, Department: "ktud", Roles: roles})
}
func TestPersistentWorkflow(t *testing.T) {
	pool, dsn := database(t)
	repo := db.New(pool)
	s := g.NewService(repo)
	owner := actor("owner", "owner", "approver")
	approver := actor("approver", "approver")
	toolOwner := actor("tools", "tool_owner")
	auditor := actor("auditor", "auditor")
	exec := func(ctx context.Context, c g.Command) g.Record {
		t.Helper()
		r, e := s.Execute(ctx, c, "test-correlation")
		if e != nil {
			t.Fatalf("%s %s: %v", c.Action, c.Kind, e)
		}
		return r
	}
	create := func(ctx context.Context, k, p, tool string, data g.Payload) g.Record {
		return exec(ctx, g.Command{Action: "create", Kind: k, Parent: p, Tool: tool, Data: data})
	}
	act := func(ctx context.Context, r g.Record, a, reason string) g.Record {
		return exec(ctx, g.Command{Action: a, Kind: r.Kind, ID: r.ID, BaseVersion: r.Version, Reason: reason})
	}
	a := create(owner, "agent", "", "", g.Payload{Name: "ktv-agent", Description: "business", Purpose: "Jira support", Risk: "medium"})
	// Existing catalog rows must actually exist; profiles do not replace the catalog.
	var n int
	if e := pool.QueryRow(context.Background(), `SELECT count(*) FROM agentregistry.agents WHERE namespace='ktud' AND name='ktv-agent'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("catalog not reused", e)
	}
	instance := create(owner, "instance", a.ID, "", g.Payload{Name: "dev", Environment: "dev", Location: "workstation"})
	if _, e := s.Execute(owner, g.Command{Action: "create", Kind: "instance", Parent: a.ID, Data: instance.Data}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatalf("duplicate instance %v", e)
	}
	request := create(owner, "request", a.ID, "", g.Payload{RequestType: "agent", Purpose: "review profile"})
	request = act(owner, request, "submit", "")
	if _, e := s.Execute(owner, g.Command{Action: "approve", Kind: "request", ID: request.ID, BaseVersion: request.Version}, ""); !errors.Is(e, g.ErrForbidden) {
		t.Fatalf("self approval %v", e)
	}
	act(approver, request, "approve", "")
	a, _ = s.Get(owner, a.ID)
	server := create(toolOwner, "server", "", "", g.Payload{Name: "jira-mcp", Tag: "v1", Description: "Metadata only"})
	tool := create(toolOwner, "tool", server.ID, "", g.Payload{Name: "create_issue", Description: "Create Jira issue", Risk: "medium", Classification: "write", Snapshot: "v1", InputSchema: []byte(`{"type":"object"}`)})
	expires := time.Now().Add(time.Hour)
	payload := g.Payload{RequestType: "access", Purpose: "create issue", Operations: []string{"create_issue"}, Constraints: map[string]string{"project": "DEMO"}, ExpiresAt: &expires}
	if _, e := s.Execute(owner, g.Command{Action: "create", Kind: "request", Parent: instance.ID, Tool: tool.ID, Data: payload}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatalf("unreviewed tool %v", e)
	}
	tool = act(approver, tool, "approve", "")
	if _, e := s.Execute(toolOwner, g.Command{Action: "update", Kind: "tool", ID: tool.ID, BaseVersion: tool.Version, Data: tool.Data}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatalf("tool mutation %v", e)
	}
	access := create(owner, "request", instance.ID, tool.ID, payload)
	access = act(owner, access, "submit", "")
	if _, e := s.Execute(auditor, g.Command{Action: "approve", Kind: "request", ID: access.ID, BaseVersion: access.Version}, ""); !errors.Is(e, g.ErrForbidden) {
		t.Fatalf("auditor write %v", e)
	}
	// Frozen payload cannot be changed through the normal mutation path.
	if _, e := s.Execute(owner, g.Command{Action: "update", Kind: "request", ID: access.ID, BaseVersion: access.Version, Data: payload}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatalf("payload tamper %v", e)
	}
	ea := auth.AuthSessionTo(context.Background(), g.Actor{ID: "ea", Name: "ea", Department: "ea", Roles: []string{"owner", "approver"}})
	if _, e := s.Get(ea, a.ID); !errors.Is(e, g.ErrNotFound) {
		t.Fatalf("cross scope get %v", e)
	}
	if _, e := s.Execute(ea, g.Command{Action: "approve", Kind: "request", ID: access.ID, BaseVersion: access.Version}, ""); !errors.Is(e, g.ErrNotFound) {
		t.Fatalf("cross scope approval %v", e)
	}
	page, e := s.List(ea, g.Filter{Kind: "agent", Limit: 1})
	if e != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("scope counts %+v %v", page, e)
	}
	// Two simultaneous decisions: exactly one decision/grant wins.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Execute(approver, g.Command{Action: "approve", Kind: "request", ID: access.ID, BaseVersion: access.Version}, "")
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, g.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("concurrent decision", success, conflicts)
	}
	grants, e := s.List(auditor, g.Filter{Kind: "grant", Parent: instance.ID})
	if e != nil || grants.Total != 1 || grants.Items[0].Tool != tool.ID {
		t.Fatalf("grant %+v %v", grants, e)
	}
	grant := grants.Items[0]
	// Decision + grant must roll back if audit persistence fails.
	second := create(owner, "request", instance.ID, tool.ID, payload)
	second = act(owner, second, "submit", "")
	broken := g.NewService(faultRepo{Repository: repo})
	if _, e := broken.Execute(approver, g.Command{Action: "approve", Kind: "request", ID: second.ID, BaseVersion: second.Version}, ""); e == nil {
		t.Fatal("audit error accepted")
	}
	still, e := s.Get(owner, second.ID)
	if e != nil || still.State != "pending_review" {
		t.Fatal("partial approval", e)
	}
	p, _ := s.List(owner, g.Filter{Kind: "grant"})
	if p.Total != 1 {
		t.Fatal("partial grant")
	}
	// Revocation and suspension never delete original decisions or restore grants.
	grant = act(approver, grant, "revoke", "no longer needed")
	a = act(approver, a, "suspend", "security review")
	if _, e := s.Execute(approver, g.Command{Action: "approve", Kind: "request", ID: second.ID, BaseVersion: second.Version}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatalf("suspend gate %v", e)
	}
	a = act(approver, a, "resume", "")
	grant, _ = s.Get(owner, grant.ID)
	if grant.State != "revoked" {
		t.Fatal("grant revived")
	}
	if _, e := s.Execute(approver, g.Command{Action: "revoke", Kind: "grant", ID: grant.ID, BaseVersion: 1, Reason: "stale"}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatal("stale revoke", e)
	}
	history, e := s.Events(auditor, g.Filter{Parent: a.ID})
	if e != nil || history.Total < 10 {
		t.Fatalf("audit descendants %d %v", history.Total, e)
	}
	// DB triggers and FKs protect history/dependencies through normal SQL privileges.
	if _, e = pool.Exec(context.Background(), `UPDATE agentregistry.mcp_servers SET spec='{}' WHERE name='jira-mcp'`); e == nil {
		t.Fatal("pinned server catalog mutated")
	}
	if _, e = pool.Exec(context.Background(), `UPDATE agentregistry.agents SET deletion_timestamp=now() WHERE name='ktv-agent'`); e == nil {
		t.Fatal("pinned agent soft-deleted")
	}
	if _, e = pool.Exec(context.Background(), `UPDATE agentregistry.governance_audit SET reason='tampered'`); e == nil {
		t.Fatal("audit mutable")
	}
	if _, e = pool.Exec(context.Background(), `UPDATE agentregistry.governance_records SET data='{}' WHERE id=$1`, access.ID); e == nil {
		t.Fatal("request snapshot mutable")
	}
	if _, e = pool.Exec(context.Background(), `DELETE FROM agentregistry.agents WHERE name='ktv-agent'`); e == nil {
		t.Fatal("catalog dependency deleted")
	}
	reopened, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	persisted, e := g.NewService(db.New(reopened)).Get(auditor, grant.ID)
	if e != nil || persisted.State != "revoked" {
		t.Fatal("restart persistence", e)
	}
	// Competing approve/reject decisions cannot both succeed.
	raceRequest := create(owner, "request", instance.ID, tool.ID, payload)
	raceRequest = act(owner, raceRequest, "submit", "")
	decisions := make(chan error, 2)
	for _, decision := range []string{"approve", "reject"} {
		go func(decision string) {
			_, e := s.Execute(approver, g.Command{Action: decision, Kind: "request", ID: raceRequest.ID, BaseVersion: raceRequest.Version, Reason: "competing decision"}, "")
			decisions <- e
		}(decision)
	}
	wins, losses := 0, 0
	for range 2 {
		e := <-decisions
		if e == nil {
			wins++
		} else if errors.Is(e, g.ErrConflict) {
			losses++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatal("approve/reject race", wins, losses)
	}
	// Server-derived expiry applies to detail, filtered count, and mutation.
	soon := time.Now().Add(time.Second)
	shortPayload := payload
	shortPayload.ExpiresAt = &soon
	short := create(owner, "request", instance.ID, tool.ID, shortPayload)
	short = act(owner, short, "submit", "")
	act(approver, short, "approve", "")
	time.Sleep(1100 * time.Millisecond)
	expired, e := s.List(auditor, g.Filter{Kind: "grant", State: "expired"})
	if e != nil || expired.Total != 1 {
		t.Fatalf("expiry query %d %v", expired.Total, e)
	}
	expiredDetail, e := s.Get(auditor, expired.Items[0].ID)
	if e != nil || expiredDetail.State != "expired" {
		t.Fatal("expiry detail", e)
	}
	if _, e = s.Execute(approver, g.Command{Action: "revoke", Kind: "grant", ID: expiredDetail.ID, BaseVersion: expiredDetail.Version, Reason: "too late"}, ""); !errors.Is(e, g.ErrConflict) {
		t.Fatal("expired mutation", e)
	}
	// Revoke also uses base versions under concurrent requests.
	grantRequest := create(owner, "request", instance.ID, tool.ID, payload)
	grantRequest = act(owner, grantRequest, "submit", "")
	act(approver, grantRequest, "approve", "")
	all, _ := s.List(owner, g.Filter{Kind: "grant"})
	var target g.Record
	for _, v := range all.Items {
		if v.Source == grantRequest.ID {
			target = v
		}
	}
	revocations := make(chan error, 2)
	for range 2 {
		go func() {
			_, e := s.Execute(approver, g.Command{Action: "revoke", Kind: "grant", ID: target.ID, BaseVersion: target.Version, Reason: "parallel revoke"}, "")
			revocations <- e
		}()
	}
	wins, losses = 0, 0
	for range 2 {
		e := <-revocations
		if e == nil {
			wins++
		} else if errors.Is(e, g.ErrConflict) {
			losses++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatal("revoke race", wins, losses)
	}

}

type faultRepo struct{ g.Repository }
type faultUnit struct{ g.Unit }

func (f faultUnit) Audit(context.Context, g.Event) error { return errors.New("injected audit failure") }
func (f faultRepo) Within(ctx context.Context, a g.Actor, fn func(g.Unit) error) error {
	return f.Repository.Within(ctx, a, func(u g.Unit) error { return fn(faultUnit{u}) })
}

func TestMigrationUpgradeRollback(t *testing.T) {
	pool, dsn := database(t)
	ctx := context.Background()
	mg, e := v1alpha1store.NewOSSMigrator(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer mg.Close()
	if e = mg.Migrate(16); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO agentregistry.agents(namespace,name,tag,spec,content_hash) VALUES('legacy','existing','v1','{"description":"preserve me"}','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`); e != nil {
		t.Fatal(e)
	}
	if e = mg.Up(); e != nil {
		t.Fatal(e)
	}
	var description string
	if e = pool.QueryRow(ctx, `SELECT spec->>'description' FROM agentregistry.agents WHERE namespace='legacy' AND name='existing'`).Scan(&description); e != nil || description != "preserve me" {
		t.Fatal("upgrade changed existing artifact", e)
	}
	if e = mg.Migrate(16); e != nil {
		t.Fatal(e)
	}
	if e = mg.Up(); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(ctx, `SELECT spec->>'description' FROM agentregistry.agents WHERE namespace='legacy' AND name='existing'`).Scan(&description); e != nil || description != "preserve me" {
		t.Fatal("rollback changed existing artifact", e)
	}
}

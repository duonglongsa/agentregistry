# Local governance demo

This demo runs PostgreSQL in Docker and runs the Go backend and Next.js UI on
the host. It does not require Kubernetes, a gateway, Keycloak, LiteLLM or an MCP
server. All approval states are Registry records only.

## One-time setup

Start only PostgreSQL and create a dedicated demo database. The database name
suffix is a startup safety check; governance local mode refuses another name.

```bash
VERSION=dev docker compose -f docker/docker-compose.yml up -d --wait postgres
docker exec agent-registry-postgres createdb \
  -U agentregistry -O agentregistry agentregistry_governance_demo
go run ./hack/governance-demo
```

The account generator is idempotent. It writes a 0600 authentication file and
random demo passwords below `.local/governance/`, which is gitignored. It never
prints passwords to server logs. Read the local credentials when assigning the
demo personas:

```bash
cat .local/governance/credentials.json
```

The configured demo accounts are `demo-long`, `demo-tool-owner`,
`demo-approver`, `demo-auditor`, `demo-admin`, and a separate-department
`demo-ea`. Names explicitly include “demo”; they do not impersonate MB users.

## Run

Terminal 1:

```bash
./scripts/governance/run.sh
```

Terminal 2:

```bash
cd ui
npm ci
npm run dev -- --hostname 127.0.0.1
```

Open <http://127.0.0.1:3000/governance>. The backend is at
`http://127.0.0.1:12121`; PostgreSQL is at `127.0.0.1:5432`. The run script
sets all safety flags explicitly. Do not bind local-demo mode to a public
interface or use its credentials outside this disposable demo database.

The `Danh mục` page detects governance mode and reads Agent, MCP Server and Tool
snapshot records through the department-scoped governance API. The legacy
upstream `/v0` catalog API is intentionally disabled in governance mode because
it does not enforce department visibility. Skills, Prompts and runtime deploy
remain upstream capabilities in public mode; they are not available through
the scoped governance catalog. A session is required to view this page in
governance mode.

Tool snapshots are reviewed directly from draft by a different Approver. The
`Hộp thư phê duyệt` page shows draft tools separately from agent-profile and
tool-access requests, which follow draft → pending_review → decision.

To stop, press Ctrl+C in the two host terminals. PostgreSQL may remain running,
or be stopped without deleting its volume:

```bash
VERSION=dev docker compose -f docker/docker-compose.yml stop postgres
```

## Seed and acceptance

Seed only the catalog/profile metadata:

```bash
python3 scripts/governance/demo.py --local-demo
```

Run the complete API acceptance scenario, including self-approval denial,
cross-department isolation, grant creation, revoke, suspend/resume and audit:

```bash
python3 scripts/governance/demo.py --local-demo --workflow
```

The script accepts only plain HTTP on loopback port 12121, verifies the server's
explicit local-demo mode, refuses datasets with non-demo ownership, and is
idempotent. It creates two department identities (KTUD and EA), `ktv-agent`, its
`dev` instance, `jira-mcp`, and the reviewed `create_issue` / `delete_issue`
metadata snapshots. It never invokes MCP, a gateway, Kubernetes or an external
backend. It does not reset or overwrite an existing database.

For a manual UI demonstration:

1. Sign in as `demo-long`. Open Agent profiles, inspect `ktv-agent` and its dev
   instance. Create an agent-profile request, submit it, and observe that the
   owner cannot approve it.
2. Sign in as `demo-approver`, approve the profile request, and verify the
   profile is approved without any tool grant.
3. Sign in as `demo-tool-owner`; create a tool snapshot. A draft tool cannot be
   selected in an access request. A separate approver reviews it.
4. As `demo-long`, create an access request for `create_issue@v1`, constrain it
   to project `DEMO`, set an expiry, and submit it. The payload hash/revision is
   displayed and cannot be edited after submission.
5. Verify `demo-long` cannot approve the request. Approve as
   `demo-approver`. The instance ledger contains only `create_issue@v1`, with
   request, revision, reviewer and decision time. `delete_issue` remains a
   reviewed catalog tool without a grant.
6. As `demo-auditor`, read the ledger and audit; mutation actions are absent and
   server calls return 403. As `demo-ea`, KTUD list/count/detail/export data is
   absent.
7. As `demo-approver`, revoke the grant with a reason. Approval provenance
   remains, while revocation actor/time/reason are added. Suspend and resume the
   agent; the revoked grant remains revoked.
8. Restart the backend and refresh the browser. The records and audit remain in
   PostgreSQL. The banner continues to state that runtime enforcement is not
   connected.

## Verification commands

The integration tests create and drop their own disposable databases beneath
the trusted local PostgreSQL server. They do not reset the demo database.

```bash
GOVERNANCE_TEST_DATABASE_URL='postgres://agentregistry:agentregistry@localhost:5432/agentregistry?sslmode=disable' \
  go test -race -count=1 \
  ./internal/registry/database/governance \
  ./internal/registry/governance \
  ./internal/registry/api/handlers/governance \
  ./internal/registry

go test -tags=unit ./internal/registry/... \
  ./pkg/registry/auth/... ./pkg/registry/v1alpha1store/...

go test ./pkg/registry/v1alpha1store -run TestMigrationsLint
git diff --check

cd ui
npm run generate
npx tsc --noEmit
npm test -- --reporter=dot
npx eslint app/governance components/governance lib/governance-api.ts
npm run build:export
```

The database tests cover validation/uniqueness, upgrade and rollback from
migration 016, scope-aware pagination/counts, stale revisions, concurrent
approve/reject/revoke, expiry, self-approval, auditor denial, department
isolation, immutable snapshots/audit, dependency protection, transactional
rollback and persistence after reopening the pool. The Huma HTTP test exercises
the real cookie/session and PostgreSQL path and verifies alternate upstream
routes are blocked in governance mode.

### Verified evidence (28 October 2026, Asia/Ho_Chi_Minh)

- Race-enabled Go governance suites passed for the database repository, domain
  service, Huma transport and composition root.
- Registry/auth/store regression unit suites passed, including upstream router,
  controller and compatibility handler tests.
- Migration lint passed. Upgrade/rollback testing preserved a pre-017 Agent
  catalog row, then applied both 017 and 018 again.
- UI generation, strict TypeScript, focused ESLint, 44 Vitest cases and the
  Next.js static export passed. Static routes include `/governance`.
- The opt-in demo workflow passed against the running backend and PostgreSQL.
  A browser run used distinct Owner, Approver and Auditor sessions, approved
  only `create_issue@v1`, restarted the backend, observed the same grant, and
  revoked it with approval and revocation provenance intact. No external
  gateway, Kubernetes, LiteLLM or MCP request was made.
- `git diff --check` passed. No commit, push, PR or upstream merge was made.

## Trusted boundary and limits

Direct PostgreSQL credentials are trusted administrator access. Do not give
them to owners, approvers or auditors. Application audit is append-only for
normal application/SQL roles; it does not claim protection against a database
superuser.

`approved`, `revoked`, `suspended`, `retired` and `expired` describe Registry
decisions. They do not issue or revoke tokens, alter a gateway, terminate a
session, call a tool, or enforce access in an external system.

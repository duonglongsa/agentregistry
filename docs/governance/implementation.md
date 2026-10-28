# Registry governance implementation

> Quyền được ghi nhận tại Registry; chưa tích hợp kiểm soát thực thi.
>
> Approved, revoked, suspended and retired are Registry decisions. They do not
> change gateway policies, tokens, external sessions, MCP execution or backend access.

## Baseline and actual stack

- Checkout: `main`, baseline `85f11fb8f24628dca3665ca3c875c2b4cb2eee7d`.
- `go.mod`: Go 1.26.9. The local verification toolchain is Go 1.27.3.
- `ui/package.json`: Next.js ^16.0.9, React ^18.2.0; lockfile: Next.js 16.2.9.
- PostgreSQL/pgx, Huma/OpenAPI, existing Next.js/Tailwind/shadcn components, Vitest.
- No new module or npm dependency. LICENSE/NOTICE are unchanged.
- No push, PR, upstream merge or production operation is part of this work.

## Reused entities and extension points

`cmd/server` → `pkg/registry.App` → `internal/registry.App` remains the startup path.
`internal/registry/registry_app.go` wires the governance repository, service,
authentication and Huma transport explicitly. The existing HTTP server, port,
OpenAPI generator, Next.js navigation and component system are reused.

Agent and MCP server registration inserts the **existing**
`agentregistry.agents` / `agentregistry.mcp_servers` tagged catalog records.
Governance profiles reference their UUIDs using restrictive FKs; they are not
another artifact/version catalog. The namespace is the authenticated department.
An existing name/tag is never overwritten or claimed automatically: collisions
return 409. Existing unowned upstream catalog artifacts stay intact and require
an explicit future administrative adoption flow; no ownership is inferred.

New migrations follow the existing embedded OSS migration runner:

- `017_governance`: records, accounts, audit, scoped indexes, dependency FKs,
  uniqueness, immutable snapshots and audit triggers.
- `018_governance_catalog_pins`: protect the semantic contents and soft deletion
  of catalog tags referenced by governance, including after switching back to
  public mode. New tags remain possible; status-only updates remain possible.

`governance_records` is a typed relationship ledger with `kind` constraints:

| Kind | Meaning | Reference |
| --- | --- | --- |
| agent | Owner, department, purpose, business description, risk, profile state | Existing tagged Agent UUID |
| instance | Environment, execution location, optional endpoint and identity metadata | Agent profile |
| server | Pinned server tag and endpoint metadata | Existing tagged MCPServer UUID |
| tool | Tool name, schema, read/write class, risk, immutable snapshot version | Server governance reference |
| request | Profile-review or tool-access request and frozen payload hash | Profile or instance; optional tool |
| grant | Approved access snapshot, approval provenance and revocation metadata | Instance, exact tool snapshot, unique source request |

The kind is immutable. Composite FKs prevent cross-department relationships.
Instance uniqueness is `(agent, environment, instance name)`. Tool uniqueness is
`(server reference, snapshot version, tool name)`; tool names are not global IDs.
Accounts have stable IDs, departments, bcrypt hashes and a revisioned role list.
No token, runtime secret or external client credential is provisioned or stored.

## Layers and transactions

- Domain/service and validation: `internal/registry/governance`.
- SQL only: `internal/registry/database/governance`; injected via Repository/Unit
  and IdentityStore interfaces. Huma handlers have no SQL or transaction logic.
- Transport: `internal/registry/api/handlers/governance`.
- UI: `/governance`, supporting an agent's profile, instances, reviews and ledger,
  tool metadata forms, an approvals inbox, audit search/export, and admin roles.
  Existing catalog AgentDetail links to governance.
- Client: `ui/lib/governance-api.ts` wraps the **generated** typed SDK with a
  same-origin cookie client. Generated files are produced by the existing tools.

Mutations take a PostgreSQL transaction advisory lock per department, then read
scoped rows `FOR UPDATE`. Versions must match `baseVersion`. The lock serializes
approval with suspension, retirement and revocation across processes/replicas.
There are no in-memory locks for business decisions. A request decision, its
unique grant and its audit event commit together; audit failure rolls everything
back. A repeated decision with a stale version returns 409. Owners cannot create
grants or set approval fields using mass assignment. Relationship and ownership
fields cannot be reassigned by update commands.

Profile and tool metadata can be edited in draft. Requests can be edited in draft;
submitted payloads cannot be edited. A replacement after submission is a new
request, not an amendment to an old decision. A profile review binds its target
metadata digest; an access request binds its tool digest/version and environment.
Changing a reviewed tool requires a new snapshot and review. References are
metadata, not claims about a tool's behavior.

## Lifecycle

- Profile: `draft → pending_review → approved`; rejection/cancellation returns
  the profile to draft. `approved → suspended → approved`; archive means
  `retired`. Sensitive reasons are required for suspend/archive.
- Requests: `draft → pending_review → approved | rejected | cancelled`;
  draft can also be cancelled. `requestType=agent` and `requestType=access` are
  distinct. Approving a profile creates no tool grant.
- Tools: draft → approved/rejected, then optional archive. The creator cannot
  review its own tool. There is no network discovery or MCP invocation.
- Grants: approved → revoked, or server-derived expired. Expiry is evaluated at
  the same captured server time in list/count/detail/mutation. No scheduler is
  needed. Revoke preserves approval actor/time, the source decision, and records
  separate revocation actor/time/reason. It never deletes history.
- Access submission/approval requires an approved profile and tool and a
  non-archived server. Suspension/retirement blocks new grants. Resume does not
  modify grants or revive expired/revoked decisions. Existing approved grants
  remain historical approvals; they are not runtime-effective permissions.
- Catalog deletion and soft deletion with dependencies are blocked; governance
  archive/retire preserves referenced records and audit.

## Authentication, authorization and alternate paths

Public upstream mode remains the default when governance is disabled. New demo
commands explicitly set `AGENT_REGISTRY_GOVERNANCE_ENABLED=true`. Invalid flag
values fail startup. Enabled governance has **no allow-all fallback**.

This delivery implements a **local demo authentication mode only**. Startup
requires all of:

- `AGENT_REGISTRY_GOVERNANCE_LOCAL_DEMO=true`;
- loopback server bind, and a loopback PostgreSQL URL whose database ends in
  `_governance_demo`;
- a 0600 auth configuration file with bcrypt cost >=12, unique stable IDs,
  valid roles/departments and a random signing key of at least 32 bytes;
- no custom HTTP/auth/database factories or external secret store.

Passwords are generated during setup. Accounts are provisioned idempotently
without resetting existing roles/passwords. Configured actor metadata is trusted
setup input; HTTP request bodies/headers never choose an actor, department or
role. Login accepts account ID/password only. Sessions are HMAC signed, expire
in two hours and use HttpOnly, SameSite=Strict, path-scoped cookies. Cookie
`Secure=false` is intentional for **loopback HTTP only**; this mode must not be
put behind a publicly reachable proxy. Mutations require an allowlisted local
Origin (3000 or the bound backend origin); no cross-origin credential CORS is
configured. Login has a process-local attempt cap. This is not an enterprise
SSO, hardened internet-facing login, password recovery or MFA implementation.

The auth provider uses the existing `auth.Session` context. Every mutation checks
server roles/ownership; the repository constrains rows to the principal's
**department in SQL**, before filtering, count and pagination. The same scope is
used by detail, audit and export. All roles may read their department's records;
owners mutate only owned profiles/instances/requests, and tool owners only their
server/tool entries. Approvers review their department, never their own requests.
Admin role does not bypass self approval. Auditors have no mutation role.

Admins may assign roles to other already provisioned accounts in their own
department, using an account base version. Self role changes and department
changes are refused. Role changes and account provisioning are audited. Role
changes invalidate old signed sessions; mutations recheck the account revision
under the department transaction lock. Password changes/account deletion and
cross-department role delegation are intentionally not exposed.

In governance mode:

- The generic upstream authorizer is deny-all (`RegistryBoundary`).
- `/v0*`, `/admin*`, non-governance `/api/*`, MCP, plugin marketplace, metrics
  and logging routes are blocked at the HTTP boundary. This covers CLI-over-HTTP,
  apply/delete/batch, runtime/deploy and compatibility read paths with unscoped
  lists. Compatibility flags are forcibly off, including configured prefixes.
- The standalone MCP port is forcibly off. Deployment, plugin and skill
  background controllers do not start. Existing runtime capabilities are not
  removed from public mode and are not activated by the governance demo.
- Static assets, documentation and health remain available. OpenAPI schemas do
  not contain user records. The public catalog/deployed UI links remain, but their
  data APIs are deliberately unavailable in this mode; use `/governance`.

Direct-DB CLI and PostgreSQL credentials are a **trusted administrator boundary**.
Do not distribute DB credentials to owners, approvers or auditors. HTTP RBAC and
application audit triggers do not control PostgreSQL superusers.

## API

All governance endpoints use `/api/governance` on the existing Huma API:

| Endpoint | Use |
| --- | --- |
| POST login / logout | Local signed session / clear cookie |
| GET me / mode | Current stable actor and explicit local demo mode |
| GET records | kind, parent, state, name/ID search, limit/offset; scoped total |
| GET records/{id} | Scoped detail |
| POST commands | create/update/submit/approve/reject/cancel/revoke/suspend/resume/archive |
| GET audit | actor/action/result, object/agent/instance subtree, UTC from/until, pagination |
| GET audit.csv | Same predicates/scope, formula-safe CSV; bounded export |
| GET accounts | Admin-only scoped account metadata (never password hashes) |
| POST accounts/{id}/roles | Admin-only, revisioned role change |

Mutation command: `action`, `kind`, optional `id`, `baseVersion`, `parent`, `tool`,
`reason`, typed `data`. Fields for decision actors, ownership, department, grant
source or record state are never accepted as client-controlled mutation fields.
Reasons are required for reject/revoke/suspend/archive. Metadata is limited to
64 KiB; tool schemas to 32 KiB and 32 schema nesting levels. JSON Schema references
are resolved locally with no network loader. Endpoints cannot include embedded
credentials/query strings/fragments. Model catalog exists upstream but model
access governance is a secondary feature and is not added here.

Audit contains UTC timestamps, stable actor ID, action/object/parent, department,
revision, reason/result, generated correlation ID and redacted before/after
hashes. Mutation audit is transactional; sensitive rejected actions are separately
persisted. Application code exposes no audit update/delete API; PostgreSQL
triggers reject ordinary update/delete. This is application append-only history,
not tamper-proof storage against a database administrator. No SIEM/ELK is used.

## Regeneration

```bash
go run ./hack/tools/gen-openapi -output openapi.yaml
(cd ui && npm run generate)
```

Do not edit the generated client by hand. Runtime Huma registration is enabled
only with governance; generator registration always includes the governance schema.

## Limits and deferred secondary work

- No gateway/Kubernetes/Keycloak/LiteLLM/MCP/backend integration or runtime enforcement.
- No external SaaS/partner control workflow. External/SaaS profiles may be recorded with
  `registrationScope=outside_mb`; instance registration and review are refused for
  them. No external control case or workflow is created.
- Production auth integration/deployment is not delivered; local mode refuses
  non-loopback bindings/databases. The configured demo accounts are not MB identities.
- No ownership/department transfer endpoint; no implicit adoption of existing
  unowned catalog tags. These operations are denied rather than mass-assigned.
- No bulk tool JSON import/preview, dashboard or model-access workflow. These are
  secondary work; normal validated tool forms and audit CSV are provided.
- Tool/catalog entries are entered manually; schema validation is not behavior verification.
- The department advisory lock favors correctness and demo simplicity over very
  high mutation throughput. Reads remain concurrent.
- Audit CSV is a bounded, scoped export with a server-time upper watermark; it
  is not a long-running analytics snapshot or an external archival system.

See [local-demo.md](local-demo.md) for setup, acceptance and test evidence.

---
task: "0007"
slug: create-agent-flow
granularity: slice
version: 0.1.0
status: draft
declares:
  - type: handler
    name: create-agent-flow
    path: internal/handlers/
scope:
  - internal/handlers/agents.go
  - internal/handlers/supabase_agent_store.go
  - internal/server/server.go
  - tests/integration/0007__create-agent-flow_test.go
---

# Task 0007 — `create-agent-flow`

> Handler `POST /v1/agents` que soporta 4 fuentes de creación (blank, template, ai-assist, github). El agente resultante queda scoped a org+grupo desde su nacimiento: fila en `agents` + objeto `CLAUDE.md` en Storage bajo `houston/{org_id}/{group_id}/agents/{agent_id}/`.

## What

An HTTP handler for `POST /v1/agents` in `internal/handlers/agents.go` that:

1. Reads `TenantContext` (org_id, group_id, role) from context via `middleware.TenantFromContext` — guaranteed present by the preceding middleware chain.
2. Reads the caller's JWT from context via `auth.JWTFromContext` — used for all Supabase calls (never service_role).
3. Decodes and validates the JSON request body:
   ```json
   {
     "name":        "my-agent",
     "source":      "blank|template|ai-assist|github",
     "template_id": "sales",
     "github_url":  "https://github.com/org/repo",
     "ai_prompt":   "optional hint for ai-assist"
   }
   ```
   `name` and `source` are always required. `template_id` is required when `source=template`. `github_url` is required when `source=github`.
4. Generates a new `uuid.UUID` for the agent ID before any external call.
5. Produces CLAUDE.md content according to `source`:
   - **`blank`**: a fixed skeleton string (12 lines, defined in spec §How).
   - **`template`**: fetches `templates/{template_id}/CLAUDE.md` from Supabase Storage via `AgentStore.GetTemplate`.
   - **`ai-assist`**: fetches the org's Anthropic key via `AgentStore.GetAnthropicKey`, then calls the Claude API (`claude-haiku-4-5-20251001`, one-shot user message, max 512 tokens) to generate CLAUDE.md content.
   - **`github`**: fetches the raw `CLAUDE.md` from `{github_url}` using the GitHub raw content URL pattern; if not found at root, returns 422.
6. Calls `AgentStore.CreateAgent` to INSERT the row into `agents` (org_id, group_id from TenantContext) and upload `CLAUDE.md` to Storage path `houston/{org_id}/{group_id}/agents/{agent_id}/CLAUDE.md`.
7. Returns `201 {"id": "<uuid>"}` on success.
8. Returns structured error envelopes (RFC §4.11) on all failure paths.

The handler itself contains no HTTP client code. All Supabase I/O goes through the `AgentStore` interface, which Task 0007 also provides as a concrete `SupabaseAgentStore` in `internal/handlers/supabase_agent_store.go`.

`internal/server/server.go` is modified: `POST /v1/agents` is re-wired from `stubHandler` to `handlers.CreateAgent(store)`, with `RequireRole(RoleManager)` (raised from the current `RoleMember` stub level).

## Why

- **PRD §3 — demo requirement:** The core demo flow is "create an agent → run it". Without `POST /v1/agents` working end-to-end, there is no agent to run and Task 0008 cannot be demonstrated.
- **RFC §4.6 — create-agent flow:** The RFC specifies exactly four creation sources with their data flows. This task is the direct implementation of that spec section.
- **RFC §4.10 (critical) — never service_role on user-facing paths:** Every Supabase call in this handler uses the caller's JWT. RLS policies on `agents` and Storage ensure org+group isolation without any application-layer filtering. The handler never reads a service_role key.
- **RFC §4.5 — RBAC:** Creating agents requires `group:manager` or higher. A `group:member` cannot create agents for their own group. Enforced by `RequireRole(RoleManager)` in the middleware chain, not duplicated in the handler.
- **RFC §4.14 — hexagonal layout:** `AgentStore` interface is defined in `internal/handlers/` (consumer-owned), keeping the handler layer free of direct Supabase SDK imports. The concrete `SupabaseAgentStore` is an adapter in the same package.

## How

### Files created / modified

| File | Action | Purpose |
|---|---|---|
| `internal/handlers/agents.go` | Create | `AgentStore` interface, `CreateAgent` handler, request/response types |
| `internal/handlers/supabase_agent_store.go` | Create | Concrete `SupabaseAgentStore` implementing `AgentStore` |
| `internal/server/server.go` | Modify | Wire `POST /v1/agents` to `CreateAgent`, raise role to `RoleManager` |
| `tests/integration/0007__create-agent-flow_test.go` | Create | Integration tests (httptest.Server stubs for Supabase) |

### Key types and signatures

```go
// internal/handlers/agents.go

package handlers

// AgentStore is the single port for all storage and DB operations in this handler.
// Implemented by SupabaseAgentStore; can be stubbed in tests.
type AgentStore interface {
    // CreateAgent inserts the agent row into DB and uploads claudeMD to Storage.
    // Storage path: houston/{orgID}/{groupID}/agents/{agentID}/CLAUDE.md
    CreateAgent(ctx context.Context, jwt string, a AgentRecord, claudeMD []byte) error

    // GetTemplate fetches templates/{templateID}/CLAUDE.md from Storage.
    // Returns ErrTemplateNotFound if the object does not exist.
    GetTemplate(ctx context.Context, jwt string, templateID string) ([]byte, error)

    // GetAnthropicKey calls /rpc/get_org_anthropic_key with the user's JWT.
    // Returns ErrNoCredentials if the org has no key.
    GetAnthropicKey(ctx context.Context, jwt string, orgID uuid.UUID) (string, error)
}

var (
    ErrTemplateNotFound = errors.New("template not found")
    ErrNoCredentials    = errors.New("org has no anthropic key")
)

type AgentRecord struct {
    ID      uuid.UUID `json:"id"`
    OrgID   uuid.UUID `json:"org_id"`
    GroupID uuid.UUID `json:"group_id"`
    Name    string    `json:"name"`
    Source  string    `json:"source"`
    Config  any       `json:"config,omitempty"`
}

type createAgentRequest struct {
    Name       string `json:"name"`
    Source     string `json:"source"`
    TemplateID string `json:"template_id"`
    GitHubURL  string `json:"github_url"`
    AIPrompt   string `json:"ai_prompt"`
}

// CreateAgent returns an http.HandlerFunc for POST /v1/agents.
// store is injectable so integration tests can substitute a stub.
func CreateAgent(store AgentStore) http.HandlerFunc
```

```go
// internal/handlers/supabase_agent_store.go

package handlers

type SupabaseAgentStore struct {
    SupabaseURL string
    AnonKey     string
}

func NewSupabaseAgentStore(supabaseURL, anonKey string) *SupabaseAgentStore

// CreateAgent implementation:
//   1. POST {SupabaseURL}/rest/v1/agents   (Authorization: Bearer jwt, apikey: AnonKey)
//      Body: AgentRecord as JSON
//   2. POST {SupabaseURL}/storage/v1/object/houston/{orgID}/{groupID}/agents/{agentID}/CLAUDE.md
//      Authorization: Bearer jwt, apikey: AnonKey, Content-Type: text/plain
// Both use auth.NewUserClient(jwt). Never the service_role key.

// GetTemplate implementation:
//   GET {SupabaseURL}/storage/v1/object/templates/{templateID}/CLAUDE.md
//   Authorization: Bearer jwt, apikey: AnonKey
//   Returns ErrTemplateNotFound on 404.

// GetAnthropicKey implementation:
//   POST {SupabaseURL}/rest/v1/rpc/get_org_anthropic_key
//   Authorization: Bearer jwt, apikey: AnonKey
//   Body: {}
//   Returns ErrNoCredentials on empty response or 404.
```

### Blank CLAUDE.md skeleton

```markdown
# Agent: {name}

> Created by Houston 2.0.

## Role

Describe the agent's role here.

## Instructions

1. Step one.
2. Step two.
```

`{name}` is interpolated with the `name` field from the request.

### ai-assist Claude API call

Model: `claude-haiku-4-5-20251001`. One-shot user message:

```
Generate a CLAUDE.md file for a Claude Code agent named "{name}".
{ai_prompt if non-empty}
Output only the CLAUDE.md file content, no explanations.
```

Max tokens: 512. Temperature: default. Uses the org's Anthropic key from `org_credentials` (decrypted server-side by the `get_org_anthropic_key()` security-definer function).

### github source

Construct raw URL: if `github_url` is `https://github.com/{owner}/{repo}`, fetch:
```
https://raw.githubusercontent.com/{owner}/{repo}/HEAD/CLAUDE.md
```
HTTP GET with 10-second timeout. 404 → 422 response. Non-2xx status → 502.

### server.go modification

Replace the current `POST /v1/agents` binding:

```go
// Before (Task 0006 stub):
mux.Handle("POST /v1/agents", protected)  // protected wraps stubHandler at RoleMember

// After (Task 0007):
store := handlers.NewSupabaseAgentStore(cfg.SupabaseURL, cfg.AnonKey)
agentProtected := auth.AuthMiddleware(authCfg)(
    middleware.TenantMiddleware(q)(
        middleware.RequireRole(middleware.RoleManager)(
            handlers.CreateAgent(store),
        ),
    ),
)
mux.Handle("POST /v1/agents", agentProtected)
```

Other v1 routes remain at `RoleMember` via the shared `protected` chain.

### Error responses

| Condition | Status | code |
|---|---|---|
| Missing `name` or `source` | 400 | `invalid_request` |
| Invalid `source` value | 400 | `invalid_request` |
| Missing `template_id` when source=template | 400 | `invalid_request` |
| Missing `github_url` when source=github | 400 | `invalid_request` |
| Template not found in Storage | 404 | `not_found` |
| GitHub URL 404 | 422 | `unprocessable` |
| GitHub URL non-2xx | 502 | `upstream_error` |
| Org has no anthropic key | 402 | `payment_required` |
| DB or Storage write error | 500 | `internal_error` |
| Role < manager | 403 | `forbidden` (from middleware) |
| No valid JWT | 401 | `unauthenticated` (from middleware) |

All error bodies follow RFC §4.11: `{"error":{"code":"...","message":"...","request_id":"..."}}`.

## Acceptance criteria

1. `POST /v1/agents` with valid `RoleManager` JWT and `{"name":"test","source":"blank"}` returns `201` with body `{"id":"<non-empty uuid>"}`. Storage stub receives an upload request for path matching `houston/{org_id}/{group_id}/agents/{uuid}/CLAUDE.md` containing the skeleton text `# Agent: test`.

2. `POST /v1/agents` with valid `RoleMember` JWT (below manager) returns `403` with `{"error":{"code":"forbidden",...}}`. Handler is never called.

3. `POST /v1/agents` with `{"name":"t","source":"template","template_id":"sales"}` and a Storage stub that returns a fake template body returns `201`. The DB stub receives `CreateAgent` with `source="template"` and the claudeMD bytes matching the stub's template response.

4. `POST /v1/agents` with `{"name":"t","source":"template"}` (missing `template_id`) returns `400` with `code:"invalid_request"`.

5. `POST /v1/agents` with `{"name":"t","source":"template","template_id":"missing"}` where the Storage stub returns 404 returns `404` with `code:"not_found"`.

6. `POST /v1/agents` with `{"name":"t","source":"ai-assist"}` and a stub that provides a valid anthropic key returns `201`. The generated CLAUDE.md is uploaded to Storage (verified via stub capture).

7. `POST /v1/agents` with `{"name":"t","source":"ai-assist"}` where `GetAnthropicKey` returns `ErrNoCredentials` returns `402` with `code:"payment_required"`.

8. `POST /v1/agents` with `{"name":"t","source":"github","github_url":"https://github.com/foo/bar"}` and a GitHub httptest.Server returning a `CLAUDE.md` body returns `201`. Uploaded content matches the stub's response body.

9. `POST /v1/agents` with `source=github` and GitHub stub returning 404 returns `422` with `code:"unprocessable"`.

10. `POST /v1/agents` with body missing `name` returns `400` with `code:"invalid_request"`.

11. `POST /v1/agents` with `source:"invalid"` returns `400` with `code:"invalid_request"`.

## Out of scope

- **Running the agent** — `POST /v1/runs` is Task 0008. This task only creates the agent record and uploads its initial files.
- **Multiple files from templates or GitHub** — MVP uploads only `CLAUDE.md`. Extending to `.houston/` directory and `houston.json` is post-MVP.
- **Template listing** — `GET /v1/templates` is not part of this task.
- **Agent update / delete** — post-MVP CRUD.
- **Storage RLS policies** — defined in Task 0003. This handler assumes the bucket and policies exist; it calls Storage as the authenticated user and trusts RLS to enforce isolation.
- **`get_org_anthropic_key` SQL function** — defined in Task 0002. This handler calls it via PostgREST RPC; the SQL is out of scope here.
- **Full RBAC matrix testing** — Task 0012.

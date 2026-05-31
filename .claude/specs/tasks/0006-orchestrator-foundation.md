---
task: "0006"
slug: orchestrator-foundation
granularity: slice
version: 0.1.0
status: implemented
declares:
  - type: server
    name: orchestrator-foundation
    path: cmd/server/
scope:
  - cmd/server/
  - internal/server/
  - tests/unit/0006__orchestrator-foundation.test.go
---

# Task 0006 — `orchestrator-foundation`

> Go HTTP server con las rutas `/v1/*` (shape Houston-compatible), propagación de request context (auth + tenant), structured logging, y el wiring de los middlewares de auth (Task 0004) y RBAC (Task 0005). Incluye el adaptador concreto de `TenantQuerier` (Supabase PostgREST) diferido por Task 0005.

## What

A runnable Go binary in `cmd/server/` that:

1. Reads configuration from environment variables (`SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_JWT_SECRET`, `SERVER_PORT`, `LOG_LEVEL`) using `os.Getenv`. No external config library.
2. Starts an HTTP server on `SERVER_PORT` (default `8080`) using stdlib `net/http` with Go 1.22 enhanced `ServeMux` (`METHOD /path` pattern matching).
3. Registers the following routes:
   - `GET /health` — returns `200 {"status":"ok"}`, no auth required.
   - `POST /v1/agents` — stub handler, auth + RBAC gated (`RequireRole(RoleMember)`).
   - `GET /v1/agents/{id}` — stub handler, auth + RBAC gated.
   - `POST /v1/runs` — stub handler, auth + RBAC gated (`RequireRole(RoleMember)`).
   - `GET /v1/runs/{id}` — stub handler, auth + RBAC gated.
4. Applies the middleware chain to every `/v1/*` route: `AuthMiddleware` (Task 0004) → `TenantMiddleware` (Task 0005) → `RequireRole` → handler.
5. Provides the concrete `TenantQuerier` implementation (`supabaseQuerier`) that POSTs to `/rpc/current_tenant` using `NewUserClient(jwt)` from Task 0004 — this is the piece Task 0005 deferred.
6. Emits structured JSON logs for every request using `log/slog` (stdlib, Go 1.21+): method, path, status, duration_ms, request_id.
7. Generates a `request_id` (UUID v4) per request and stores it in the request context so middleware and handlers can read it.
8. Shuts down gracefully on `SIGINT`/`SIGTERM` with a 30-second drain timeout.

Stub handlers return `200 {"status":"ok","stub":true}` and will be replaced by Tasks 0007–0009.

## Why

- **Workflow dependency graph:** Tasks 0007–0012 all depend on a running HTTP server with auth + RBAC middleware wired. Without Task 0006, no subsequent task has an integration test surface.
- **RFC §4.10 (critical):** The concrete `supabaseQuerier` is the only place in the codebase where a network call is made with the user's JWT to `/rpc/current_tenant`. Isolating it here ensures the "never service_role on user-facing paths" invariant is easy to audit.
- **RFC §4.14 (hexagonal layout):** The server wires all adapters together (HTTP, Supabase). Tasks 0004 and 0005 define interfaces; Task 0006 provides the concrete implementations and the composition root.
- **PRD §3 (demo requirement):** The demo requires a curl-drivable server. This task produces the first runnable binary — everything before it (0001–0005) is library/schema code that cannot be demoed without a server.
- **go.mod version:** bumped from 1.21 to 1.22 to use enhanced ServeMux pattern matching (no external router needed). Go 1.22 is the minimum version for this task.

## How

### Files created

| File | Purpose |
|---|---|
| `cmd/server/main.go` | Entry point: config loading, server init, graceful shutdown |
| `internal/server/server.go` | `Server` struct, route registration, middleware wiring |
| `internal/server/requestid.go` | `RequestIDMiddleware` — generates UUID, stores in context, adds to response header |
| `internal/server/logging.go` | `LoggingMiddleware` — wraps ResponseWriter to capture status, logs via slog |
| `internal/server/supabase_querier.go` | `supabaseQuerier` — concrete `TenantQuerier` that POSTs to `/rpc/current_tenant` |
| `internal/server/health.go` | `healthHandler` |
| `internal/server/stubs.go` | Stub handlers for `/v1/agents` and `/v1/runs` |
| `.env.example` | Add `LOG_LEVEL=info` entry (append to file created by Task 0004) |
| `tests/unit/0006__orchestrator-foundation_test.go` | Unit + integration tests (created by QA before coder) |

### Key types and signatures

```go
// cmd/server/main.go
type Config struct {
    SupabaseURL    string
    AnonKey        string
    JWTSecret      string
    Port           string // default "8080"
    LogLevel       string // default "info"
}

func loadConfig() Config // reads os.Getenv, fills defaults
func main()              // loadConfig → New(cfg) → srv.Run() → graceful shutdown
```

```go
// internal/server/server.go
type Server struct {
    cfg    Config
    mux    *http.ServeMux
    logger *slog.Logger
}

func New(cfg Config) *Server
func (s *Server) Run(ctx context.Context) error // blocks until ctx cancelled or signal
```

```go
// internal/server/supabase_querier.go
// supabaseQuerier satisfies middleware.TenantQuerier.
// It calls POST /rpc/current_tenant with Authorization: Bearer <jwt>
// using auth.NewUserClient(jwt) — never the service_role key.
type supabaseQuerier struct {
    supabaseURL string
}

func newSupabaseQuerier(supabaseURL string) *supabaseQuerier
func (q *supabaseQuerier) CurrentTenant(ctx context.Context, userID uuid.UUID) (middleware.TenantContext, error)
// Returns middleware.ErrNoMembership when the RPC returns an empty array.
```

```go
// internal/server/requestid.go
type requestIDKey struct{}
func RequestIDFromContext(ctx context.Context) string
func RequestIDMiddleware(next http.Handler) http.Handler
```

### Middleware chain (per /v1/* route)

```
RequestIDMiddleware
  → LoggingMiddleware
    → AuthMiddleware(cfg)          // internal/auth — 401 if no valid JWT
      → TenantMiddleware(querier)  // internal/middleware — 401/403 if no membership
        → RequireRole(minRole)     // internal/middleware — 403 if role insufficient
          → handler
```

### go.mod change

Bump `go 1.21` → `go 1.22` to enable `"GET /v1/agents/{id}"` wildcard syntax in `http.ServeMux`.

### supabaseQuerier — PostgREST call

```
POST <SUPABASE_URL>/rest/v1/rpc/current_tenant
Authorization: Bearer <user_jwt>
apikey: <SUPABASE_ANON_KEY>
Content-Type: application/json
Body: {}

Success response: [{"org_id":"<uuid>","group_id":"<uuid>","role":"<string>"}]
Empty response []: return ErrNoMembership
```

## Acceptance criteria

1. `GET /health` returns `200 {"status":"ok"}` without any `Authorization` header.
2. `POST /v1/agents` without an `Authorization` header returns `401` with `{"error":{"code":"unauthenticated",...}}`.
3. `POST /v1/agents` with a valid JWT but a `supabaseQuerier` stub returning `ErrNoMembership` returns `403` with `{"error":{"code":"forbidden",...}}`.
4. `POST /v1/agents` with a valid JWT, stub querier returning `TenantContext{Role: RoleMember}`, and `RequireRole(RoleMember)` on the route returns `200 {"status":"ok","stub":true}`.
5. `POST /v1/agents` with a valid JWT, stub querier returning `TenantContext{Role: RoleMember}`, and `RequireRole(RoleManager)` on the route returns `403`.
6. The server starts on `SERVER_PORT=18080` (test port) when the env var is set; `GET http://localhost:18080/health` returns 200.
7. Every response to `/v1/*` includes an `X-Request-ID` header with a non-empty UUID string.
8. The JSON log line emitted for each request contains the fields: `level`, `msg`, `method`, `path`, `status`, `duration_ms`, `request_id`.
9. `supabaseQuerier.CurrentTenant` makes a `POST` to `/rest/v1/rpc/current_tenant` with `Authorization: Bearer <jwt>` and `apikey: <anon_key>` headers; verified in unit test with `httptest.Server`.
10. `go build ./cmd/server/` succeeds on Go 1.22+.

## Out of scope

- **Actual agent/run CRUD handlers** — Tasks 0007 (agent-registry), 0008 (run-executor), 0009 (credential-vault). Stubs only here.
- **Database migrations** — Tasks 0001 (db-schema) and 0002 (rls-postgres). The server never applies migrations at startup.
- **Storage layout** — Task 0003.
- **Full RBAC test matrix** — Task 0012 (rbac-test). This task only wires the middleware; exhaustive role × operation testing is Task 0012's scope.
- **Token refresh / re-authentication** — post-MVP. The server accepts only a current valid JWT per request.
- **TLS / HTTPS** — local-only MVP. The server listens on plain HTTP.

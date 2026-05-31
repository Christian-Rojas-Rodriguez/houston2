---
task: "0005"
slug: rbac-middleware
granularity: slice
version: 0.1.0
status: draft
declares:
  - type: middleware
    name: rbac-middleware
    path: internal/middleware/
scope:
  - internal/middleware/
  - tests/unit/0005__rbac-middleware.test.go
---

# Task 0005 — `rbac-middleware`

> Go middleware que deriva `(org_id, group_id, role)` del usuario autenticado consultando `memberships` via `current_tenant()`, y hace enforcement de 403 cuando la operación solicitada supera el rol del usuario.

## What

An HTTP middleware layer in `internal/middleware/` that, for every authenticated request:

- Reads the caller's `user_id` from the request context (placed there by Task 0004's auth handler).
- Calls the `TenantQuerier.CurrentTenant` method — backed at runtime by a Supabase security-definer call to `current_tenant()` — to retrieve `(org_id, group_id, role)` for that user.
- Attaches a `TenantContext` value to the request context so downstream handlers can read org/group/role without re-querying.
- Exposes a `RequireRole(minRole)` middleware factory that a route can chain to gate access by minimum role level, returning 403 if the caller's role is below the threshold.
- Returns 401 (code `"unauthenticated"`) when no user identity is present and 403 (code `"forbidden"`) when the user has no membership row for the requested context.

No Supabase client is imported by this package. All external I/O goes through the `TenantQuerier` interface, which Task 0006 will satisfy with a concrete implementation.

## Why

- Houston 2.0 is a multi-tenant platform where every data operation must be scoped to an `(org_id, group_id)` pair and gated by the caller's role (PRD §3 — multi-tenancy, PRD §5 — authorization model). Without this middleware, any authenticated user could reach any group's data.
- The RFC places authorization enforcement at the HTTP middleware layer rather than inside individual handlers, so that the policy is applied uniformly and cannot be bypassed by adding a new route that omits a guard (RFC §4.10, RFC §7 — task granularity rationale for 0005).
- Skipping this task leaves every handler after Task 0004 unguarded: JWT validation would pass but no membership check would occur, violating the RFC's "never service_role on user-facing paths" invariant (RFC §4.10) and leaving all group data open to any logged-in user.
- This task is sized as a single vertical slice (RFC §7) because it owns exactly one concern — deriving and enforcing tenant context — and its interface boundary (`TenantQuerier`) decouples it cleanly from the Supabase client that Task 0006 provides.

## How

### Files created

| File | Purpose |
|---|---|
| `internal/middleware/role.go` | `Role` integer enum, constants, `ParseRole`, `RoleFromContext` helpers |
| `internal/middleware/interfaces.go` | `TenantQuerier` interface (consumer-owned, per RFC §4.14 hexagonal rule) |
| `internal/middleware/tenant.go` | `TenantContext` struct, `TenantMiddleware(q TenantQuerier)` factory, `TenantFromContext` |
| `internal/middleware/rbac.go` | `RequireRole(minRole Role)` middleware factory |
| `internal/middleware/errors.go` | `writeErrorJSON` helper that emits RFC §4.11 error envelope |
| `tests/unit/0005__rbac-middleware.test.go` | Unit tests (all pass without network or Supabase credentials) |

### Key types and signatures

```go
// internal/middleware/role.go
type Role int

const (
    RoleMember  Role = iota // "group:member"  — lowest privilege
    RoleManager             // "group:manager"
    RoleOwner               // "org:owner"      — highest privilege
)

func ParseRole(s string) (Role, error)
// "group:member" → RoleMember
// "group:manager" → RoleManager
// "org:owner"    → RoleOwner
// anything else  → error

// internal/middleware/interfaces.go
type TenantQuerier interface {
    CurrentTenant(ctx context.Context, userID uuid.UUID) (TenantContext, error)
}
// ErrNoMembership is returned (or wrapped) by implementations when the user
// has no row in the memberships table for the current org context.
var ErrNoMembership = errors.New("no membership")

// internal/middleware/tenant.go
type TenantContext struct {
    OrgID   uuid.UUID
    GroupID uuid.UUID
    Role    Role
}

// TenantMiddleware chains before route handlers.
// It reads auth.UserIDFromContext (Task 0004), calls q.CurrentTenant,
// stores the result, and delegates to next — or short-circuits with
// 401 / 403 as defined in Acceptance criteria 1–2.
func TenantMiddleware(q TenantQuerier) func(http.Handler) http.Handler

// TenantFromContext retrieves the TenantContext stored by TenantMiddleware.
func TenantFromContext(ctx context.Context) (TenantContext, bool)

// internal/middleware/rbac.go
// RequireRole MUST be chained after TenantMiddleware.
// If no TenantContext is found it responds 500 (internal ordering error).
func RequireRole(minRole Role) func(http.Handler) http.Handler
```

### Error envelope (RFC §4.11)

```json
{ "error": { "code": "unauthenticated", "message": "...", "request_id": "..." } }
```

`request_id` is read from context if present (placed by Task 0006's request-ID middleware); if absent a fresh UUID v4 is generated inline so the middleware works standalone.

### Design decisions

- **stdlib net/http only** — no gorilla/mux or chi dependency in this package (RFC §4.14 — keep the middleware layer framework-agnostic so Task 0006 can choose the router).
- **Consumer-owned interface** (`TenantQuerier` lives in `internal/middleware`, not in the Supabase client package) — this is the hexagonal pattern from RFC §4.14; it keeps `internal/middleware` free of any Supabase import.
- **Integer enum for roles** — comparison is `callerRole >= minRole` (one integer compare, no string set operations at enforcement time).
- **`current_tenant()` is called with the user's JWT** via the concrete querier injected by Task 0006, never the service_role key (RFC §4.10).
- **`ErrNoMembership` sentinel** — the middleware checks `errors.Is(err, ErrNoMembership)` to distinguish "user has no row" (403) from an actual DB/network error (502 or 500, left to Task 0006's error handler for now; during this task a generic 500 is acceptable for non-membership errors).

### Integration points

- **Task 0004** (auth-middleware): provides `auth.UserIDFromContext(ctx context.Context) (uuid.UUID, bool)`. `TenantMiddleware` calls this function; it must be imported from `internal/auth`.
- **Task 0002** (db-schema): defines the `current_tenant()` SQL function and the `memberships` table that the concrete querier (Task 0006) will call.
- **Task 0006** (http-server): wires the concrete `TenantQuerier` implementation and registers routes with `TenantMiddleware` and `RequireRole` chains. Nothing in Task 0005 references Task 0006.
- **Task 0012** (full RBAC matrix): may extend the `Role` enum or add a `RequirePermission` helper; the integer-enum design leaves room for this without breaking the existing middleware.

## Acceptance criteria

1. A request that reaches `TenantMiddleware` with no `user_id` in its context (i.e., `auth.UserIDFromContext` returns `false`) receives a 401 response with JSON body `{"error":{"code":"unauthenticated","message":"<any non-empty string>","request_id":"<non-empty string>"}}` and the downstream handler is never called.
2. A request that has a valid `user_id` in context but whose `TenantQuerier.CurrentTenant` call returns `ErrNoMembership` receives a 403 response with JSON body `{"error":{"code":"forbidden","message":"<any non-empty string>","request_id":"<non-empty string>"}}` and the downstream handler is never called.
3. A request with a valid `user_id` in context where `TenantQuerier.CurrentTenant` returns `TenantContext{OrgID: X, GroupID: Y, Role: RoleManager}` causes `TenantFromContext` called inside the downstream handler to return that exact `TenantContext` with `ok == true`.
4. A handler chain of `TenantMiddleware` → `RequireRole(RoleManager)` → downstream, where the stored `TenantContext.Role` is `RoleMember`, responds 403 to the request and does not call the downstream handler.
5. The same chain with `TenantContext.Role == RoleManager` calls the downstream handler and does not short-circuit (downstream can write its own 200).
6. The same chain with `TenantContext.Role == RoleOwner` calls the downstream handler and does not short-circuit (owner satisfies `>= RoleManager`).
7. A handler chain that uses `RequireRole(RoleMember)` without a preceding `TenantMiddleware` (so no `TenantContext` is in context) responds 500; this confirms the ordering guard is in place rather than silently returning 403.
8. `ParseRole("org:owner")` returns `(RoleOwner, nil)`; `ParseRole("group:manager")` returns `(RoleManager, nil)`; `ParseRole("group:member")` returns `(RoleMember, nil)`; `ParseRole("superadmin")` returns a non-nil error.
9. All tests in `tests/unit/0005__rbac-middleware.test.go` pass with `go test ./tests/unit/... -run TestRBAC` using a stub `TenantQuerier` — no network calls, no Supabase credentials, no environment variables required.
10. `go list -f '{{.Imports}}' ./internal/middleware/...` produces output that contains no import path matching `supabase`, `postgrest`, or any Supabase client package.

## Out of scope

- The concrete `TenantQuerier` implementation that calls Supabase with the user's JWT — that is Task 0006's responsibility.
- Route registration, HTTP server setup, and request-ID middleware — Task 0006.
- JWT validation and `user_id` injection into context — Task 0004.
- Multi-membership support (a user belonging to multiple orgs/groups simultaneously) — post-MVP, noted in RFC §3 non-goals.
- Logging, metrics, and structured request-ID generation as a middleware concern — Task 0006 owns the observability layer.
- The full permission matrix (beyond the three-level role ordering) — Task 0012.

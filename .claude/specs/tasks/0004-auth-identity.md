---
task: "0004"
slug: auth-identity
granularity: slice
version: 0.1.0
status: implemented
declares:
  - type: handler
    name: auth-identity
    path: internal/auth/
scope:
  - internal/auth/
  - .env.example
  - tests/unit/0004__auth-identity.test.go
---

# Task 0004 — `auth-identity`

> Supabase project propio + Google SSO PKCE + loopback redirect implementado en Go. No reusa el project de Houston (credenciales baked-in en Tauri). Establece quién es el usuario; la autorización (org/grupo/rol) la maneja Task 0005.

## What

A developer or end user running the Houston 2.0 orquestador locally can authenticate with Google via a browser and have the resulting Supabase JWT validated by the Go server. Specifically:

- `.env.example` is present at the repo root with four documented variables: `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_JWT_SECRET`, and `SERVER_PORT`. A developer can copy it to `.env` and fill in the values without reading any other file.
- The Go server exposes `GET /auth/login` which redirects the browser to the Supabase Google SSO authorization URL using PKCE (code_challenge + code_verifier). A CSRF state token is generated per request, stored in a signed cookie (`SameSite=Lax`, `HttpOnly`), and included in the redirect.
- The Go server exposes `GET /auth/callback` which: (a) validates the CSRF state cookie against the `state` query parameter, (b) exchanges the `code` query parameter with Supabase for a JWT, and (c) returns the JWT as `{ "token": "<jwt>" }` for curl-driven MVP usage.
- A reusable middleware function `AuthMiddleware` in package `internal/auth` reads the `Authorization: Bearer <token>` header, validates the JWT signature using `SUPABASE_JWT_SECRET` (HS256), rejects expired or malformed tokens with HTTP 401, and places the validated `user_id` (UUID) in the request context.
- An exported accessor `UserIDFromContext(ctx context.Context) (uuid.UUID, bool)` in `internal/auth` is the single point of contract for downstream tasks (Task 0005) to read the authenticated identity.

No RBAC, no org/group derivation, no role enforcement occurs in this task — those are Task 0005's responsibility.

## Why

- **PRD §5 P0:** "Reuso de Houston: login (Supabase Auth + Google SSO PKCE)" is a P0 requirement. Every tenant-scoped operation in Postgres and Storage depends on a verified `user_id` that maps to a row in `memberships`. Without a validated identity, RLS policies have nothing to evaluate against.
- **Why PKCE:** Implicit flow leaks tokens in the browser URL and has been deprecated by OAuth 2.1. PKCE is the correct pattern for a local loopback redirect (RFC §4.9). The CSRF state parameter + signed cookie guard against open-redirect attacks.
- **Why a dedicated Supabase project:** Houston's Supabase credentials are baked-in at Tauri build time via Vite compile-time substitution — there is no way to reuse them (RFC §4.9, RFC §5 alternatives table).
- **Why this task before 0005:** Task 0005 (rbac-middleware) has a hard dependency on Task 0004 (workflow §4 dependency graph: `AU → RB`). It consumes `UserIDFromContext` and `AuthMiddleware`. Without Task 0004, Task 0005 cannot be built or tested.
- **Why HS256 / `SUPABASE_JWT_SECRET`:** Supabase issues JWTs signed with HS256. Validating the signature is what makes the token trustworthy. Trusting claims without verification allows any client to forge a `user_id`.

## How

**Files created:**

| Path | Action |
|---|---|
| `.env.example` | Create |
| `internal/auth/auth.go` | Create — PKCE handlers |
| `internal/auth/middleware.go` | Create — `AuthMiddleware` |
| `internal/auth/context.go` | Create — `UserIDFromContext` |
| `tests/unit/0004__auth-identity.test.go` | Created by QA before coder |

**Go dependencies to declare in `go.mod`:**
- `github.com/golang-jwt/jwt/v5` — JWT parsing and HS256 validation
- `github.com/google/uuid` — UUID type for user_id
- stdlib `net/http`, `crypto/rand`, `encoding/base64` — no framework in auth layer

**`.env.example` variables:**
```
SUPABASE_URL=https://<project-ref>.supabase.co
SUPABASE_ANON_KEY=<anon-key>
SUPABASE_JWT_SECRET=<jwt-secret>
SERVER_PORT=8080
```

**Key types and function signatures:**

```go
// context.go — the contract consumed by Task 0005
type contextKey string
const userIDKey contextKey = "user_id"

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool)
```

```go
// auth.go — PKCE handlers
type Config struct {
    SupabaseURL string
    AnonKey     string
    JWTSecret   string
    Port        string // default "8080"
}

func LoginHandler(cfg Config) http.HandlerFunc    // generates PKCE + state, redirects to Supabase
func CallbackHandler(cfg Config) http.HandlerFunc // validates state, exchanges code, returns { "token": "..." }
```

```go
// middleware.go
// AuthMiddleware validates HS256 JWT from Authorization: Bearer header.
// On failure → 401 JSON: { "error": { "code": "unauthenticated", "message": "...", "request_id": "..." } }
// On success → places user_id in context, calls next.
func AuthMiddleware(cfg Config) func(http.Handler) http.Handler

// NewUserClient wraps *http.Client to inject Authorization: Bearer <jwt> on every request.
func NewUserClient(jwt string) *http.Client
```

**JWT validation rules:**
- Parse with `jwt.ParseWithClaims`, verify `alg == HS256` explicitly (reject `alg: none` or RS256).
- Extract `sub` claim as `user_id` UUID string; parse into `uuid.UUID`.
- Reject if expired (`ExpiresAt.Before(time.Now())`).

**CSRF state handling:**
- State token: random 16 bytes, base64url-encoded.
- Stored in cookie `pkce_state` (`HttpOnly: true`, `SameSite: Lax`), signed with HMAC-SHA256 over `SUPABASE_JWT_SECRET`.
- Validated on callback: cookie value must match `state` query parameter.

## Acceptance criteria

1. `.env.example` exists at the repo root with exactly `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_JWT_SECRET`, and `SERVER_PORT`, each with a placeholder value and an inline comment.

2. `GET /auth/login` responds HTTP 302. `Location` header points to `SUPABASE_URL`'s OAuth authorize endpoint and includes `response_type=code`, `code_challenge`, `code_challenge_method=S256`, and `state`. Response sets a `pkce_state` cookie with `HttpOnly` and `SameSite=Lax`.

3. `GET /auth/callback?code=<valid_code>&state=<valid_state>` with matching `pkce_state` cookie returns HTTP 200 and `{ "token": "<non-empty string>" }`.

4. `GET /auth/callback?code=<any>&state=<mismatched_state>` returns HTTP 400. No Supabase token exchange is attempted (verified by mock transport in unit test).

5. `AuthMiddleware`: a request with a valid, non-expired, HS256-signed JWT in `Authorization: Bearer` calls the next handler, and `UserIDFromContext(r.Context())` returns a non-zero `uuid.UUID` with `ok == true`.

6. `AuthMiddleware`: a request with a tampered JWT (invalid signature) returns HTTP 401 with `{ "error": { "code": "unauthenticated", ... } }`. Next handler is never called.

7. `AuthMiddleware`: a request with a valid-signature but expired JWT returns HTTP 401 with `code: "unauthenticated"`.

8. `UserIDFromContext` on a context with no user_id returns `uuid.Nil, false`.

9. `NewUserClient(jwt)` returns an `*http.Client` whose transport adds `Authorization: Bearer <jwt>` to every outbound request. Verified with `httptest.Server` in unit test.

10. `go build ./internal/auth/...` succeeds with zero errors on Go 1.21+.

## Out of scope

- **RBAC and org/group/role derivation.** Placing only `user_id` in context is the boundary. Deriving `(org_id, group_id, role)` and enforcing 403 by role are Task 0005's responsibility.
- **The `service_role` admin client.** Task 0004 defines only `NewUserClient`. The admin client is provisioned in Task 0006.
- **Session persistence or token refresh.** No cookie-based sessions, no refresh token handling. Caller stores the JWT and re-authenticates when expired. Post-MVP.
- **Multi-membership / tenant-switching.** MVP enforces `LIMIT 1` in `current_tenant()`. Post-MVP.
- **Production redirect URIs or HTTPS.** Loopback only. Cloud auth config is out of scope for local-only MVP.
- **UI login screens.** Demo is curl-driven (PRD §3 non-goals). No HTML login page.

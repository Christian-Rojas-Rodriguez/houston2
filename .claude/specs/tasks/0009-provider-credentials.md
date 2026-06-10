---
task: "0009"
slug: provider-credentials
granularity: slice
version: 0.1.0
status: draft
branch: feat/0009-provider-credentials
declares:
  - type: handler
    name: provider-credentials
    path: internal/handlers/credentials.go
scope:
  - internal/handlers/credentials.go
  - tests/unit/handlers/0009__provider-credentials_test.go
---

# Task 0009 — `provider-credentials`

> CRUD de API keys de Anthropic por org: `POST /v1/orgs/{id}/credentials` (registrar), `PUT` (rotar), `DELETE` (revocar), `GET` (status booleano). Solo accesible para `org:owner`. La key se almacena cifrada en `org_credentials` via la capa de store y se inyecta como `ANTHROPIC_API_KEY` en cada run.

## What

Producir el archivo `internal/handlers/credentials.go` con:

1. Interfaz `CredentialStore` — puerto hexagonal, mismo patrón que `AgentStore`.
2. Cuatro handler factories:
   - `RegisterCredential(store CredentialStore) http.HandlerFunc` — `POST /v1/orgs/{id}/credentials`
   - `RotateCredential(store CredentialStore) http.HandlerFunc` — `PUT /v1/orgs/{id}/credentials`
   - `RevokeCredential(store CredentialStore) http.HandlerFunc` — `DELETE /v1/orgs/{id}/credentials`
   - `GetCredentialStatus(store CredentialStore) http.HandlerFunc` — `GET /v1/orgs/{id}/credentials`
3. Suite TDD en `tests/unit/handlers/0009__provider-credentials_test.go` — 9 casos, sin red ni credenciales de Supabase.

Todas las rutas requieren rol `org:owner` (configurado en server.go, fuera del scope de esta task). Los handlers agregan defensa en profundidad verificando que `{id}` del path coincide con `tc.OrgID`.

## Why

- **BYOA (Bring Your Own API key)**: Task 0008 (`run-agent-flow`) inyecta `ANTHROPIC_API_KEY` en el subproceso `claude` leyendo la key vía `CredentialStore`. Sin la key configurada por el `org:owner`, ningún run puede ejecutarse. Esta task cierra el loop de onboarding.
- **Aislamiento de keys**: la key de Anthropic es un secret de alta sensibilidad. RLS de `org_credentials` (Task 0002, `credentials_owner` policy) garantiza que solo el `org:owner` puede leer/escribir la fila. Los handlers validan además que `{id}` del path coincide con `tc.OrgID` del TenantContext (defensa en profundidad).
- **Nunca exponer la key en respuestas**: el `GET` devuelve solo un booleano (`has_key`). Ningún endpoint retorna el valor en claro ni el valor cifrado. La key se propaga únicamente al store.
- **Formato validado en la frontera**: la firma `sk-ant-` es el único formato válido de Anthropic. Rechazar en la frontera evita persistir basura en `org_credentials`.
- **Cifrado en la capa de store, no en el handler**: el handler recibe y valida plaintext. El store concreto (`SupabaseCredentialStore`, fuera del scope de esta task) llama a `pgp_sym_encrypt` via PostgREST RPC o el upsert directo con la columna `bytea`.

## How

### Interfaz `CredentialStore`

```go
// CredentialStore is the port for all credential storage operations.
// Defined in the consumer package (internal/handlers) following the hexagonal pattern
// established by AgentStore.
type CredentialStore interface {
    // UpsertCredential persists the plaintext key for the given org (insert or update).
    // Encryption is the store's responsibility — the handler passes plaintext.
    UpsertCredential(ctx context.Context, jwt string, orgID uuid.UUID, plaintextKey string) error

    // DeleteCredential removes the org_credentials row for the given org.
    DeleteCredential(ctx context.Context, jwt string, orgID uuid.UUID) error

    // HasCredential reports whether the org has a key configured, without exposing it.
    HasCredential(ctx context.Context, jwt string, orgID uuid.UUID) (bool, error)
}
```

### Handler factories and response contracts

| Factory | Method | Path | Status OK | Body OK |
|---|---|---|---|---|
| `RegisterCredential` | POST | `/v1/orgs/{id}/credentials` | 201 Created | `{"org_id":"<uuid>","has_key":true}` |
| `RotateCredential` | PUT | `/v1/orgs/{id}/credentials` | 200 OK | `{"org_id":"<uuid>","has_key":true}` |
| `RevokeCredential` | DELETE | `/v1/orgs/{id}/credentials` | 204 No Content | (empty body) |
| `GetCredentialStatus` | GET | `/v1/orgs/{id}/credentials` | 200 OK | `{"org_id":"<uuid>","has_key":<bool>}` |

### Validation logic for POST and PUT

```
1. TenantContext present → 500 if missing (middleware ordering bug)
2. Parse {id} path param → 400 "invalid_request" if not a valid UUID
3. {id} == tc.OrgID → 403 "forbidden" if mismatch (defense in depth; RLS also blocks)
4. Decode JSON body → 400 "invalid_request" if malformed
5. body.key present and non-empty → 400 "invalid_request" if absent
6. strings.HasPrefix(key, "sk-ant-") → 400 "invalid_request" if prefix missing
7. store.UpsertCredential(ctx, jwt, orgID, key) → 500 "internal_error" if fails
8. Write success response (201 or 200)
```

### Validation logic for DELETE

```
1. TenantContext present → 500 if missing
2. Parse {id} path param → 400 if not a valid UUID
3. {id} == tc.OrgID → 403 if mismatch
4. store.DeleteCredential(ctx, jwt, orgID) → 500 if fails
5. 204 No Content, empty body
```

### Validation logic for GET

```
1. TenantContext present → 500 if missing
2. Parse {id} path param → 400 if not a valid UUID
3. {id} == tc.OrgID → 403 if mismatch
4. store.HasCredential(ctx, jwt, orgID) → 500 if fails
5. 200 OK, {"org_id": "<uuid>", "has_key": <bool>}
```

### Key codebase patterns to follow

- **Error envelope**: `writeHandlerError(w, r, status, code, message)` — defined in `internal/handlers/agents.go`. Reuse it (it lives in the same `handlers` package).
- **TenantContext**: `middleware.TenantFromContext(r.Context())` returns `(TenantContext, bool)`.
- **JWT**: `auth.JWTFromContext(r.Context())` — always present after `AuthMiddleware`.
- **Path param**: `r.PathValue("id")` — Go 1.22 enhanced ServeMux.
- **Package**: `package handlers` — same package as `agents.go`.

### Integration with server.go (out of scope for this task)

```go
credStore := handlers.NewSupabaseCredentialStore(cfg.SupabaseURL, cfg.AnonKey)
ownerChain := func(h http.Handler) http.Handler {
    return auth.AuthMiddleware(authCfg)(
        middleware.TenantMiddleware(q)(
            middleware.RequireRole(middleware.RoleOwner)(h),
        ),
    )
}
mux.Handle("POST /v1/orgs/{id}/credentials",   ownerChain(handlers.RegisterCredential(credStore)))
mux.Handle("PUT /v1/orgs/{id}/credentials",    ownerChain(handlers.RotateCredential(credStore)))
mux.Handle("DELETE /v1/orgs/{id}/credentials", ownerChain(handlers.RevokeCredential(credStore)))
mux.Handle("GET /v1/orgs/{id}/credentials",    ownerChain(handlers.GetCredentialStatus(credStore)))
```

This wiring is the responsibility of the next sprint or the task that updates server.go.

### Dependencies

- **Requires**: Task 0002 (`rls-postgres`) — `org_credentials` with RLS active (`credentials_owner` policy).
- **Requires**: Task 0005 (`rbac-middleware`) — `middleware.RequireRole`, `middleware.TenantContext`, `middleware.RoleOwner`.
- **Requires**: Task 0006 (`orchestrator-foundation`) — `writeHandlerError`, Go 1.22 ServeMux path values.
- **Unblocks**: Task 0008 (`run-agent-flow`) — can obtain the key via `CredentialStore.HasCredential` / the concrete store's `GetAnthropicKey`.

## Acceptance criteria

### AC-1: POST with valid key (`sk-ant-*`) → 201 Created, `has_key: true`

`POST /v1/orgs/{orgID}/credentials` with body `{"key":"sk-ant-api03-xxx"}` returns 201 and body `{"org_id":"<uuid>","has_key":true}`. `UpsertCredential` is called exactly once with the plaintext key. Response `Content-Type: application/json`.

### AC-2: POST with invalid key (no `sk-ant-` prefix) → 400 `invalid_request`

Body `{"key":"invalid-format"}`. Response 400, `error.code == "invalid_request"`. `UpsertCredential` is not called.

### AC-3: PUT with valid key → 200 OK, `has_key: true`

Same contract as AC-1 but HTTP status 200 via PUT.

### AC-4: PUT with invalid key → 400 `invalid_request`

Same contract as AC-2 but via PUT.

### AC-5: DELETE → 204 No Content

`DELETE /v1/orgs/{orgID}/credentials` returns 204 with empty body. `DeleteCredential` is called exactly once.

### AC-6: GET when key exists → 200 OK, `has_key: true`

`GET /v1/orgs/{orgID}/credentials`. `HasCredential` mock returns `true`. Response 200, `{"org_id":"<uuid>","has_key":true}`.

### AC-7: GET when key does not exist → 200 OK, `has_key: false`

`HasCredential` mock returns `false`. Response 200, `{"org_id":"<uuid>","has_key":false}`.

### AC-8: Path org ID != TenantContext org ID → 403 `forbidden`

The `{id}` path param does not match `tc.OrgID`. Response 403, `error.code == "forbidden"`. Store methods are not called.

### AC-9: No response contains the plaintext key

The 201 and 200 response bodies do not include any substring that starts with `sk-ant-`. The plaintext key is passed only to the store mock, never serialized in the response.

## Out of scope

- `SupabaseCredentialStore` (concrete store implementation) — implemented when server.go integrates the handlers (separate task or next sprint).
- Wiring in `server.go` — responsibility of a separate task.
- SQL migration for `upsert_org_credential()` RPC — if the concrete store requires a new RPC, that generates a new migration (separate task).
- OAuth relay (BYOA with browser redirect) — post-MVP.
- Multi-key support (multiple providers per org) — post-MVP.

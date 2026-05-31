---
task: "0009"
slug: provider-credentials
granularity: slice
version: 0.1.0
status: done
declares:
  - type: handler
    name: provider-credentials
    path: internal/handlers/credentials.go
scope:
  - internal/handlers/credentials.go
  - tests/unit/handlers/0009__provider-credentials_test.go
---

# Task 0009 — `provider-credentials`

> CRUD de API keys de Anthropic por org: `POST /v1/orgs/{id}/credentials` (registrar), `PUT` (rotar), `DELETE` (revocar). Solo accesible para `org:owner`. La key se almacena cifrada en `org_credentials` y se inyecta como `ANTHROPIC_API_KEY` en cada run.

## What

Producir handlers Go para:
- `POST /v1/orgs/{id}/credentials` — registrar API key. Valida formato (`sk-ant-`). Cifra antes de persistir. Solo `org:owner` (403 si no).
- `PUT /v1/orgs/{id}/credentials` — rotar key existente. Mismas validaciones.
- `DELETE /v1/orgs/{id}/credentials` — revocar key.
- `GET /v1/orgs/{id}/credentials` — retorna solo si la key está configurada (`{"org_id": "…", "has_key": bool}`), nunca el valor en claro.

RLS de `org_credentials` garantiza que un `org:owner` solo puede operar sobre su propio org. Los handlers agregan defensa en profundidad verificando que el `{id}` del path coincide con el `org_id` del `TenantContext`.

Artefactos:

1. `internal/handlers/credentials.go` — interfaz `CredentialStore`, cuatro handler factories (`RegisterCredential`, `RotateCredential`, `RevokeCredential`, `GetCredentialStatus`).
2. `tests/unit/handlers/0009__provider-credentials_test.go` — suite TDD, 9 casos, sin red ni credenciales de Supabase.

## Why

- **BYOA (Bring Your Own API key)**: Task 0008 (`run-agent-flow`) inyecta `ANTHROPIC_API_KEY` en el subproceso `claude`. Sin la key configurada por el `org:owner`, ningún run puede ejecutarse. Esta task cierra el loop de onboarding.
- **Aislamiento de keys**: la key de Anthropic es un secret de alta sensibilidad. RLS de `org_credentials` (Task 0002, AC-10) garantiza que solo el `org:owner` puede leer/escribir la fila. Los handlers validan además que `{id}` del path coincide con el `org_id` del tenant autenticado (defensa en profundidad).
- **Jamás exponer la key en respuestas**: el `GET` devuelve solo un booleano (`has_key`). Ningún endpoint retorna el valor en claro, ni el valor cifrado.
- **Formato validado en la frontera**: la firma `sk-ant-` es el único formato válido de Anthropic. Rechazar en la frontera evita persistir basura en `org_credentials`.

## How

### Interfaz `CredentialStore`

```go
type CredentialStore interface {
    // UpsertCredential persiste la key (insert o update) para el org dado.
    // El cifrado lo maneja la capa de store; el handler recibe plaintext.
    UpsertCredential(ctx context.Context, jwt string, orgID uuid.UUID, plaintextKey string) error

    // DeleteCredential elimina la fila de org_credentials para el org dado.
    DeleteCredential(ctx context.Context, jwt string, orgID uuid.UUID) error

    // HasCredential reporta si el org tiene una key configurada (sin exponerla).
    HasCredential(ctx context.Context, jwt string, orgID uuid.UUID) (bool, error)
}
```

### Handler factories

| Factory | Method | Path | Status OK | Body OK |
|---|---|---|---|---|
| `RegisterCredential` | POST | `/v1/orgs/{id}/credentials` | 201 Created | `{"org_id":"…","has_key":true}` |
| `RotateCredential` | PUT | `/v1/orgs/{id}/credentials` | 200 OK | `{"org_id":"…","has_key":true}` |
| `RevokeCredential` | DELETE | `/v1/orgs/{id}/credentials` | 204 No Content | (vacío) |
| `GetCredentialStatus` | GET | `/v1/orgs/{id}/credentials` | 200 OK | `{"org_id":"…","has_key":bool}` |

### Lógica de validación (POST y PUT)

```
1. TenantContext presente → 500 si no (middleware ordering bug)
2. Parsear {id} del path → 400 si no es UUID válido
3. {id} == tc.OrgID → 403 si no (defensa en profundidad; RLS también lo bloquearía)
4. JSON body.key presente → 400 si falta
5. strings.HasPrefix(key, "sk-ant-") → 400 si no
6. store.UpsertCredential → 500 si falla
```

### Integración con server.go (fuera del scope de esta task)

El archivo `internal/server/server.go` debe agregar:
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

Esta integración es responsabilidad del siguiente sprint (o de la task que actualice server.go).

### Dependencias

- **Requiere**: Task 0002 (`rls-postgres`) — `org_credentials` con RLS activo.
- **Requiere**: Task 0005 (`rbac-middleware`) — `middleware.RequireRole`, `middleware.TenantContext`.
- **Desbloquea**: Task 0008 (`run-agent-flow`) puede obtener la key vía `CredentialStore.GetAnthropicKey`.

## Acceptance criteria

### AC-1: POST con key válida (`sk-ant-*`) → 201 Created, `has_key: true`

`POST /v1/orgs/{orgID}/credentials` con body `{"key":"sk-ant-api03-xxx"}` retorna 201 y body `{"org_id":"<uuid>","has_key":true}`. `UpsertCredential` se llama exactamente una vez con la plaintext key.

### AC-2: POST con key inválida (sin prefijo `sk-ant-`) → 400 `invalid_request`

Body `{"key":"invalid-format"}`. Respuesta 400, `error.code == "invalid_request"`. `UpsertCredential` no se llama.

### AC-3: PUT con key válida → 200 OK, `has_key: true`

Mismo contrato que AC-1 pero status 200.

### AC-4: PUT con key inválida → 400 `invalid_request`

Mismo contrato que AC-2 pero via PUT.

### AC-5: DELETE → 204 No Content

`DELETE /v1/orgs/{orgID}/credentials` retorna 204 con body vacío. `DeleteCredential` se llama exactamente una vez.

### AC-6: GET cuando la key existe → 200 OK, `has_key: true`

`GET /v1/orgs/{orgID}/credentials`. `HasCredential` retorna `true`. Respuesta 200, `has_key: true`.

### AC-7: GET cuando la key no existe → 200 OK, `has_key: false`

`HasCredential` retorna `false`. Respuesta 200, `has_key: false`.

### AC-8: Org ID del path ≠ org ID del TenantContext → 403 `forbidden`

El `{id}` del path no coincide con `tc.OrgID`. Respuesta 403 antes de llamar al store.

### AC-9: Ninguna respuesta contiene la plaintext key

El body de 201 y 200 no incluye ningún substring que sea la key enviada ni empieza con `sk-ant-`. La key solo se propaga al store, nunca se serializa en la respuesta.

## Out of scope

- OAuth relay completo (BYOA con browser redirect) — post-MVP.
- `SupabaseCredentialStore` (implementación concreta): fuera del scope de tests unitarios de esta task; se implementa cuando server.go integre los handlers.
- Wiring en `server.go` — responsabilidad de una task separada o del siguiente sprint.
- Migración SQL para `upsert_org_credential()` RPC — si el store concreto requiere un nuevo RPC, genera una nueva migración (task separada).

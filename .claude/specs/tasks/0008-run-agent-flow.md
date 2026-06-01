---
task: "0008"
slug: run-agent-flow
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: handler
    name: run-agent-flow
    path: internal/handlers/runs.go
scope:
  - internal/handlers/runs.go
  - internal/handlers/supabase_run_store.go
  - internal/runtime/
  - internal/server/server.go
  - .env.example
  - tests/integration/0008__run-agent-flow_test.go
---

# Task 0008 — `run-agent-flow`

> Handler `POST /v1/agents/{id}/runs` — baja el contexto del agente desde Supabase Storage a un workspace aislado por run (`/tmp/houston-run-{uuid}/`), **levanta una sesión real de Claude Code (`claude`) como subprocess local** con la API key inyectada, captura la salida, y persiste el `run`. Soporta runs concurrentes.

> **Decisión de alcance (gate, 2026-05-31):** la API key sale del **entorno del server** (`ANTHROPIC_API_KEY`, vía `.env`), NO del `org_credentials` cifrado. El path BYOA per-org (RPC `get_org_anthropic_key` + vault) **no existe** y queda como deuda de producción (out of scope). Esto hace que el MVP "crear → correr" funcione hoy con una key real + el binario `claude` instalado local.

## What

`POST /v1/agents/{id}/runs` (gate `RoleMember` — cualquier rol puede correr agentes de su grupo, RFC §4.5) que:

1. Lee `TenantContext` (org/group/role) y el JWT del caller del contexto.
2. Decodifica el body: `{"prompt": "<texto>"}` (`prompt` requerido → 400 `invalid_request` si falta).
3. **Verifica acceso al agente**: lee la fila de `agents` con `{id}` usando el JWT del caller (RLS garantiza que solo ve agentes de sus grupos visibles). Si no existe/visible → 404 `not_found`. Obtiene `org_id`/`group_id` del agente.
4. **INSERT** en `runs` `{agent_id, org_id, group_id, user_id: caller, prompt, status: 'pending'}` → obtiene `run_id`.
5. Ejecuta el run (síncrono para el MVP — el cliente espera el resultado; la concurrencia entre runs distintos se da por workspace único + el manejo per-request de Go):
   - UPDATE `runs.status = 'running'`.
   - Crea workspace `/tmp/houston-run-{run_id}/`.
   - **Hidrata el workspace** descargando de Storage (con el JWT del caller, RLS en efecto) todos los objetos bajo `houston/{org}/{group}/agents/{agent}/` (al menos `CLAUDE.md`) y `houston/{org}/general/` si existe.
   - **Corre `claude` como subprocess** en el workspace, en modo no-interactivo (print), con `ANTHROPIC_API_KEY=<key del server>` en el env y el `prompt`. Captura stdout.
   - UPDATE `runs.result = <stdout>, status = 'done'` (o `status = 'error'` + el error en `result` si el subprocess falla).
   - Limpia el workspace (incluso ante error/panic — `defer`).
6. Retorna `200 {"run_id": "<uuid>", "status": "done", "result": "<stdout>"}` (o el envelope de error RFC §4.11).

La API key se lee de `server.Config.AnthropicAPIKey` (env `ANTHROPIC_API_KEY`). Si está vacía → el run falla con `502 upstream_error` / `runs.status='error'` y un mensaje accionable ("ANTHROPIC_API_KEY no configurada en el server").

## Why

- **PRD §3 — el demo del MVP es "crear un agente → correrlo".** 0007 ya crea; 0008 es la otra mitad. Sin esto no hay MVP demostrable.
- **RFC §4.7 — run-agent flow:** prescribe exactamente este flujo (workspace aislado por run, contexto desde Storage, `claude` subprocess, API key inyectada, concurrencia).
- **Aislamiento:** el contexto se baja con el **JWT del caller** (RLS de Storage en efecto, ya corregida por R-STORAGE) — un run nunca puede hidratar contexto de otro tenant. El workspace por-UUID evita que runs concurrentes compartan filesystem.
- **Key desde el env del server (MVP):** el usuario tiene una key y quiere correr `claude` ya. El cifrado per-org (`org_credentials`) es el diseño de producción (BYOA) y queda diferido — no bloquea el demo.

## How

### Archivos

| Archivo | Acción |
|---|---|
| `internal/handlers/runs.go` | Handler `CreateRun` + interfaces `RunStore` y `Runner` (consumer-owned, patrón de `AgentStore`) + tipos request/response |
| `internal/handlers/supabase_run_store.go` | Concrete `SupabaseRunStore`: GetAgent / CreateRun / UpdateRun / DownloadContext vía PostgREST + Storage REST con el JWT del caller |
| `internal/runtime/runner.go` | Concrete `ClaudeRunner` (implementa `Runner`): `os/exec` de `claude` en print-mode con `ANTHROPIC_API_KEY` + prompt, captura stdout |
| `internal/runtime/workspace.go` | Crear/limpiar `/tmp/houston-run-{uuid}/`; escribir los objetos descargados |
| `internal/server/server.go` | Wire `POST /v1/agents/{id}/runs` (RoleMember) + agregar `AnthropicAPIKey` a `Config` (env `ANTHROPIC_API_KEY`) |
| `.env.example` | Documentar `ANTHROPIC_API_KEY` |
| `tests/integration/0008__run-agent-flow_test.go` | Tests con `Runner` y `RunStore` mockeados (sin `claude` real ni Supabase real) |

### Interfaces (hexagonal, mockeables)

```go
// internal/handlers/runs.go
type RunStore interface {
    // GetAgent verifica acceso (RLS) y retorna org/group del agente; ErrAgentNotFound si no visible.
    GetAgent(ctx context.Context, jwt string, agentID uuid.UUID) (AgentRef, error)
    // CreateRun INSERTa runs(pending) y retorna el run_id.
    CreateRun(ctx context.Context, jwt string, r RunRecord) (uuid.UUID, error)
    // UpdateRun setea status (+ result) del run.
    UpdateRun(ctx context.Context, jwt string, runID uuid.UUID, status, result string) error
    // DownloadContext baja a destDir los objetos de Storage del agente (+ general).
    DownloadContext(ctx context.Context, jwt string, a AgentRef, destDir string) error
}

type Runner interface {
    // Run ejecuta `claude` en workspaceDir con apiKey + prompt; retorna stdout o error.
    Run(ctx context.Context, workspaceDir, prompt, apiKey string) (string, error)
}

// CreateRun(store RunStore, runner Runner, apiKey string) http.HandlerFunc
```

`ClaudeRunner` real: `exec.CommandContext(ctx, "claude", "--print", prompt)` con `cmd.Dir = workspaceDir` y `cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+apiKey)`; `cmd.Output()` captura stdout. (El flag exacto de modo no-interactivo lo confirma el coder contra la versión de `claude` instalada; el contrato del `Runner` lo abstrae.)

### Wiring (server.go)

Espejo de `agentProtected` (0007), pero a `RoleMember`:
```go
runStore := handlers.NewSupabaseRunStore(cfg.SupabaseURL, cfg.AnonKey)
runner := runtime.NewClaudeRunner()
runProtected := auth.AuthMiddleware(authCfg)(
    middleware.TenantMiddleware(q)(
        middleware.RequireRole(middleware.RoleMember)(
            handlers.CreateRun(runStore, runner, cfg.AnthropicAPIKey),
        )))
mux.Handle("POST /v1/agents/{id}/runs", runProtected)
```
`Config` gana `AnthropicAPIKey string` leído de `os.Getenv("ANTHROPIC_API_KEY")` en `LoadConfig`.

### Concurrencia

Cada request corre en su goroutine (HTTP server de Go) y usa un workspace `/tmp/houston-run-{run_id}/` único → sin races de filesystem. El run es **síncrono** respecto del request (el cliente espera el resultado). Async/polling (`GET /v1/runs/{id}`) es post-MVP.

### Tests

Patrón de `tests/integration/0007__create-agent-flow_test.go`: `mockRunStore` (captura INSERT/UPDATE, devuelve AgentRef fijo) + `mockRunner` (devuelve stdout fijo o error) + `httptest`. Cubrir: happy path (201/200 + result), agente no visible → 404, prompt faltante → 400, member puede correr (no 403), runner error → status error + 502, key vacía → error accionable. **Sin** `claude` real ni Supabase real (eso es verificación manual / acceptance).

## Acceptance criteria

1. **AC1 — happy path**: `POST /v1/agents/{AgentID1}/runs` con `{"prompt":"hola"}` y un `Runner` mock que devuelve `"output X"` → **200** con `{"run_id":…,"status":"done","result":"output X"}`. El store recibe CreateRun(pending) y UpdateRun(done, "output X").
2. **AC2 — RBAC member permitido**: el mismo request con un JWT `group:member` → NO 403 (la ruta es `RoleMember`).
3. **AC3 — agente no visible**: `{id}` de un agente fuera de los grupos del caller → **404** `not_found`; no se crea run.
4. **AC4 — prompt faltante**: body sin `prompt` → **400** `invalid_request`; no se crea run.
5. **AC5 — contexto desde Storage**: el flujo invoca `DownloadContext` con el `AgentRef` correcto antes de `Runner.Run`, y `Runner.Run` recibe el `workspaceDir` hidratado (verificado vía mock que el dir contiene el `CLAUDE.md` descargado).
6. **AC6 — runner error**: si `Runner.Run` retorna error → el run queda `status='error'` (result = mensaje), respuesta **502** `upstream_error`; el workspace se limpia igual.
7. **AC7 — key inyectada**: `Runner.Run` recibe la `apiKey` de `Config.AnthropicAPIKey`; con key vacía → error accionable (no se cuelga).
8. **AC8 — workspace aislado + cleanup**: el run usa `/tmp/houston-run-{run_id}/` único y lo borra al terminar (incluso ante error). Dos runs concurrentes no comparten dir.
9. **AC9 — wiring**: `POST /v1/agents/{id}/runs` está ruteado en `server.go` (no 404), a `RoleMember`; sin JWT → 401.
10. **AC10 — suite default verde**: `go test ./...` pasa con los tests de 0008 (mocks, sin deps reales).

## Out of scope

- **Credenciales per-org cifradas (BYOA producción)**: `org_credentials` + RPC `get_org_anthropic_key` + vault key → deuda diferida. El MVP usa `ANTHROPIC_API_KEY` del env del server.
- **Async / streaming**: el run es síncrono; polling `GET /v1/runs/{id}` y streaming de tokens (WebSocket) → post-MVP.
- **Reintentos / timeouts avanzados / cancelación de runs** → post-MVP (un `context` timeout básico es suficiente).
- **Instalar/pinnear el binario `claude`** → asumido presente en el PATH del server (riesgo R3 del workflow).
- **Tests de acceptance con `claude` real** → verificación manual local; los tests automáticos mockean el `Runner`.

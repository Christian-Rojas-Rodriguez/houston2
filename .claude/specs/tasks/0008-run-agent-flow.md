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
  - internal/runtime/
  - tests/integration/0008__run-agent-flow.test.go
---

# Task 0008 — `run-agent-flow`

> Handler `POST /v1/agents/{id}/runs` — gather context del tenant desde Storage (RLS garantiza scope), lanza Claude Code CLI como subprocess en workspace aislado por run (`/tmp/houston-run-{uuid}/`), inyecta la API key del org, y soporta runs concurrentes sin race conditions.

## What

_Skeleton — completar con `/task-run 0008`._

Producir el handler Go para `POST /v1/agents/{id}/runs` que:
1. Verifica acceso al agente (el `org_id`/`group_id` del agente debe coincidir con el del usuario).
2. Lee la API key del org desde `org_credentials`.
3. INSERT en `runs` con `status: 'pending'`.
4. Lanza una goroutine que:
   - En paralelo: (a) crea workspace `/tmp/houston-run-{uuid}/` y lanza `claude` CLI; (b) descarga TODOS los objetos de `houston/{org_id}/{group_id}/agents/{agent_id}/` y `houston/{org_id}/general/` via Supabase Storage SDK (RLS en efecto).
   - Hidrata el workspace con los archivos descargados.
   - Envía el prompt a Claude Code. Captura stdout.
   - Limpia el workspace post-run.
5. UPDATE `runs` con `result` y `status: 'done'` (o `'error'`).
6. Retorna `200 { "result": "..." }`.

**Concurrencia:** múltiples runs simultáneos funcionan porque cada uno tiene su propio UUID de workspace.

## Why

_Skeleton — completar con `/task-run 0008`._

## How

_Skeleton — completar con `/task-run 0008`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0008`._ Streaming de tokens (WebSocket) es post-MVP.

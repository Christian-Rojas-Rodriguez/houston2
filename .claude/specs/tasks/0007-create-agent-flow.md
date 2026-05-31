---
task: "0007"
slug: create-agent-flow
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: handler
    name: create-agent-flow
    path: internal/handlers/agents.go
scope:
  - internal/handlers/agents.go
  - tests/integration/0007__create-agent-flow.test.go
---

# Task 0007 — `create-agent-flow`

> Handler `POST /v1/agents` que soporta 4 fuentes de creación (blank, template, ai-assist, github). El agente resultante queda scoped a org+grupo desde su nacimiento: fila en `agents` + objetos en Storage.

## What

_Skeleton — completar con `/task-run 0007`._

Producir el handler Go para `POST /v1/agents` que:
1. Verifica rol ≥ `group:manager` (via middleware Task 0005).
2. Según `source`:
   - `blank` → genera `CLAUDE.md` skeleton mínimo.
   - `template` → copia `templates/{template_id}/*` al prefix del org/grupo (Storage RLS).
   - `ai-assist` → genera `CLAUDE.md` via Claude API (modelo económico, one-shot, usando la API key del org de `org_credentials`).
   - `github` → fetch del `houston.json` + `CLAUDE.md` + `.houston/` del repo indicado.
3. INSERT en `agents` con `org_id`, `group_id`.
4. Upload de archivos a `houston/{org_id}/{group_id}/agents/{agent_id}/`.
5. Retorna `201 { "id": "<uuid>" }`.

## Why

_Skeleton — completar con `/task-run 0007`._

## How

_Skeleton — completar con `/task-run 0007`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0007`._ El run del agente va en Task 0008.

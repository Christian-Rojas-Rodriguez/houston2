---
task: "0006"
slug: orchestrator-foundation
granularity: slice
version: 0.1.0
status: skeleton
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

> Go HTTP server con las rutas `/v1/*` (shape Houston-compatible), propagación de request context (auth + tenant), structured logging, y el wiring de los middlewares de auth (Task 0004) y RBAC (Task 0005).

## What

_Skeleton — completar con `/task-run 0006`._

Producir:
1. Binario Go en `cmd/server/` con un HTTP server que escucha en puerto configurable.
2. Registro de rutas `/v1/*` — inicialmente con handlers stub para los paths que se implementan en Tasks 0007–0009.
3. Wiring de middleware chain: auth → RBAC → handler.
4. Structured logging (JSON, nivel configurable).
5. Health check en `GET /health`.
6. Graceful shutdown.

## Why

_Skeleton — completar con `/task-run 0006`._

## How

_Skeleton — completar con `/task-run 0006`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0006`._ Los handlers concretos van en Tasks 0007, 0008, 0009.

---
task: "0005"
slug: rbac-middleware
granularity: slice
version: 0.1.0
status: skeleton
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

_Skeleton — completar con `/task-run 0005`._

Producir middleware Go en `internal/middleware/` que:
1. Lee el `user_id` del request context (puesto por auth handler, Task 0004).
2. Consulta `current_tenant()` en Supabase para obtener `(org_id, group_id, role)`.
3. Adjunta el tenant context al request context.
4. Provee helpers de autorización: `RequireRole(minRole)` que retorna 403 si el rol del usuario es insuficiente.
5. Retorna 401 si el usuario no está autenticado, 403 si no tiene membresía en el org solicitado.

## Why

_Skeleton — completar con `/task-run 0005`._

## How

_Skeleton — completar con `/task-run 0005`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0005`._ El server HTTP y el wiring de routes van en Task 0006.

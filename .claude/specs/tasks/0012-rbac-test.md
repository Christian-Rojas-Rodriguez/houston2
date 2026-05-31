---
task: "0012"
slug: rbac-test
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: test
    name: rbac-test
    path: tests/acceptance/0012__rbac-test.go
scope:
  - tests/acceptance/0012__rbac-test.go
---

# Task 0012 — `rbac-test`

> **Acceptance gate del MVP.** Verifica que cada operación fuera del rol permitido retorna exactamente 403. Cubre todas las combinaciones rol × operación definidas en RFC §4.5.

## What

_Skeleton — completar con `/task-run 0012`._

Producir test Go en `tests/acceptance/0012__rbac-test.go` que:
1. Levanta el fixture del MVP (via Task 0010 helpers).
2. Para cada combinación prohibida (rol × operación), ejecuta la request y afirma HTTP 403:
   - `group:member` intenta CRUD de grupos → 403.
   - `group:member` intenta add/remove de members → 403.
   - `group:manager` intenta CRUD de grupos → 403.
   - `group:manager` intenta operar sobre agente de otro grupo → 403.
   - Cualquier rol intenta operar en otro org → 403 (o 404, según el diseño de no-leak).
   - `group:manager` / `group:member` intentan gestionar `org_credentials` → 403.
3. Para cada operación permitida, afirma que NO retorna 403 (200/201/204).

## Why

_Skeleton — completar con `/task-run 0012`._

## How

_Skeleton — completar con `/task-run 0012`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0012`._ El aislamiento cross-org (leak de datos) está cubierto por Task 0011.

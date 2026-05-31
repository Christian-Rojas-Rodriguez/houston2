---
task: "0011"
slug: leak-test
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: test
    name: leak-test
    path: tests/acceptance/0011__leak-test.go
scope:
  - tests/acceptance/0011__leak-test.go
  - scripts/leak-test.sh
---

# Task 0011 — `leak-test`

> **Acceptance gate del MVP.** Actuar como Usuario A (Org 1 / Grupo 1 / `group:member`) y afirmar que absolutamente ninguna row de Postgres ni objeto de Storage de Org 2, ni de Org 1/Grupo 2, es retornada. Si este test falla, el aislamiento se rompió.

## What

_Skeleton — completar con `/task-run 0011`._

Producir:
1. Test Go en `tests/acceptance/0011__leak-test.go` (o script bash `scripts/leak-test.sh`) que:
   - Levanta el fixture del MVP (via Task 0010 helpers).
   - Autentica como Usuario A (Org 1 / Grupo 1).
   - Consulta TODAS las tablas tenant-scoped: `organizations`, `groups`, `memberships`, `agents`, `runs`.
   - Consulta TODOS los objetos de Storage accesibles.
   - Afirma que ningún resultado tiene `org_id` de Org 2, ni `group_id` de Org 1/Grupo 2 (excepto el grupo `general` del propio org).
   - EXIT 0 si aislamiento íntegro; EXIT 1 con detalle del leak si no.
2. El test debe ser corrido en CI como gate de merge.

## Why

_Skeleton — completar con `/task-run 0011`._

## How

_Skeleton — completar con `/task-run 0011`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0011`._ Los tests de RBAC van en Task 0012.

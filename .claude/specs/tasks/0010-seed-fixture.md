---
task: "0010"
slug: seed-fixture
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: script
    name: seed-fixture
    path: scripts/seed.go
scope:
  - scripts/seed.go
  - tests/testdata/seed.sql
  - tests/helpers/fixture.go
---

# Task 0010 — `seed-fixture`

> Script reproducible que crea el fixture MVP: 2 orgs / 2 grupos / 3 usuarios / 3 roles (ver PRD §4). Usado por los tests de acceptance (Tasks 0011 y 0012) para tener un estado de partida determinista.

## What

_Skeleton — completar con `/task-run 0010`._

Producir:
1. Script `scripts/seed.go` (o SQL equivalente) que inserta el fixture canónico:
   - Org 1 + Grupo 1 (general) + Grupo 2
   - Org 2 + Grupo 1 (general)
   - Usuario A → Org 1 / Grupo 1 / `group:member`
   - Usuario B → Org 1 / Grupo 2 / `group:manager`
   - Usuario C → Org 2 / Grupo 1 / `org:owner`
   - Al menos un agente en Org 1/Grupo 1 y otro en Org 2/Grupo 1.
   - Al menos un objeto Storage en cada prefix de org/grupo.
2. Helper Go en `tests/helpers/fixture.go` para setup/teardown del fixture en tests.
3. Script de limpieza que elimina el fixture (idempotente).

## Why

_Skeleton — completar con `/task-run 0010`._

## How

_Skeleton — completar con `/task-run 0010`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0010`._ Los tests de acceptance que usan el fixture van en Tasks 0011 y 0012.

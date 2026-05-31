---
task: "0002"
slug: rls-postgres
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: policy
    name: rls-postgres
    path: supabase/migrations/
scope:
  - supabase/migrations/
  - tests/unit/0002__rls-postgres.test.sql
---

# Task 0002 — `rls-postgres`

> Políticas RLS en todas las tablas tenant-scoped + helper `current_tenant()` (security definer). Aquí vive el aislamiento real: ninguna query mal formada puede retornar datos de otro tenant.

## What

_Skeleton — completar con `/task-run 0002`._

Producir migraciones que:
1. Habilitan RLS en `organizations`, `groups`, `memberships`, `agents`, `runs`, `org_credentials`.
2. Crean la función `current_tenant()` (security definer) que deriva `(org_id, group_id, role)` desde `memberships` usando `auth.uid()`.
3. Crean las policies de aislamiento según RFC §4.3: invariante `org = mine AND group ∈ {general, mine}`, restricción de runs (nadie lee runs ajenos incluido `org:owner`), y restricción de `org_credentials` (solo `org:owner`).

## Why

_Skeleton — completar con `/task-run 0002`._

## How

_Skeleton — completar con `/task-run 0002`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0002`._ Storage RLS va en Task 0003.

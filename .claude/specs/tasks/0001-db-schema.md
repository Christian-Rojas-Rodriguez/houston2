---
task: "0001"
slug: db-schema
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: migration
    name: db-schema
    path: supabase/migrations/
scope:
  - supabase/migrations/
  - tests/unit/0001__db-schema.test.sql
---

# Task 0001 — `db-schema`

> Migraciones Supabase que crean las tablas base del modelo multi-tenant: `organizations`, `groups`, `memberships`, `agents`, `runs`, y `org_credentials`. Es la capa 0 de la que dependen RLS, auth, RBAC y todos los flows.

## What

_Skeleton — completar con `/task-run 0001`._

Producir las migraciones SQL de Supabase que crean las seis tablas del modelo de datos de Houston 2.0 (ver RFC §4.2). Cada tabla debe tener sus constraints, foreign keys, y `created_at` por defecto. `org_credentials.anthropic_key` debe almacenarse cifrada.

## Why

_Skeleton — completar con `/task-run 0001`._

## How

_Skeleton — completar con `/task-run 0001`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0001`._ Las políticas RLS van en Task 0002; el Storage en Task 0003.

---
task: "0003"
slug: storage-layout
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: config
    name: storage-layout
    path: supabase/storage/
scope:
  - supabase/storage/
  - scripts/seed-templates.sh
  - tests/unit/0003__storage-layout.test.sh
---

# Task 0003 — `storage-layout`

> Bucket structure de Supabase Storage, políticas RLS de Storage por prefix, y seed del template `sales` en `templates/`. Define los prefijos canónicos `houston/{org}/{group}/` y `houston/{org}/general/`.

## What

_Skeleton — completar con `/task-run 0003`._

Producir:
1. Configuración de bucket(s) en Supabase Storage con los prefijos canónicos del RFC §4.4.
2. Storage RLS policies: `houston/{org_id}/{group_id}/` y `houston/{org_id}/general/` scoped al tenant; `templates/` read-only para todos los usuarios autenticados.
3. Script `scripts/seed-templates.sh` que sube al menos un template `sales` con un `CLAUDE.md` mínimo al prefix `templates/sales/`.

## Why

_Skeleton — completar con `/task-run 0003`._

## How

_Skeleton — completar con `/task-run 0003`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0003`._ La lógica de copiar templates al crear agentes va en Task 0007.

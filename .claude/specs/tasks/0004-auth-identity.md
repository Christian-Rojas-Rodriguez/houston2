---
task: "0004"
slug: auth-identity
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: handler
    name: auth-identity
    path: internal/auth/
scope:
  - internal/auth/
  - .env.example
  - tests/unit/0004__auth-identity.test.go
---

# Task 0004 — `auth-identity`

> Supabase project propio + Google SSO PKCE + loopback redirect implementado en Go. No reusa el project de Houston (credenciales baked-in en Tauri). Establece quién es el usuario; la autorización (org/grupo/rol) la maneja Task 0005.

## What

_Skeleton — completar con `/task-run 0004`._

Producir:
1. Configuración de Supabase project propio (documentada en `.env.example`: `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_JWT_SECRET`).
2. Instrucciones/script para configurar Google OAuth en GCP Console y registrar el provider en Supabase Auth (redirect URI: `http://localhost:{PORT}/auth/callback`).
3. Handler Go en `internal/auth/` que maneja el PKCE exchange: recibe el código de callback, intercambia por JWT de Supabase, y adjunta el `user_id` al request context.

## Why

_Skeleton — completar con `/task-run 0004`._

## How

_Skeleton — completar con `/task-run 0004`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0004`._ La derivación de `(org_id, group_id, role)` y el enforcement de roles van en Task 0005.

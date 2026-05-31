---
task: "0009"
slug: provider-credentials
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: handler
    name: provider-credentials
    path: internal/handlers/credentials.go
scope:
  - internal/handlers/credentials.go
  - tests/unit/0009__provider-credentials.test.go
---

# Task 0009 — `provider-credentials`

> CRUD de API keys de Anthropic por org: `POST /v1/orgs/{id}/credentials` (registrar), `PUT` (rotar), `DELETE` (revocar). Solo accesible para `org:owner`. La key se almacena cifrada en `org_credentials` y se inyecta como `ANTHROPIC_API_KEY` en cada run.

## What

_Skeleton — completar con `/task-run 0009`._

Producir handlers Go para:
- `POST /v1/orgs/{id}/credentials` — registrar API key. Valida formato (`sk-ant-...`). Cifra antes de persistir. Solo `org:owner` (403 si no).
- `PUT /v1/orgs/{id}/credentials` — rotar key existente. Mismas validaciones.
- `DELETE /v1/orgs/{id}/credentials` — revocar key.
- `GET /v1/orgs/{id}/credentials` — retorna solo si la key está configurada (boolean), nunca el valor en claro.

RLS de `org_credentials` garantiza que un `org:owner` solo puede operar sobre su propio org.

## Why

_Skeleton — completar con `/task-run 0009`._

## How

_Skeleton — completar con `/task-run 0009`._

## Acceptance criteria

_Por escribir — QA antes de implementación._

## Out of scope

_Por definir durante `/task-run 0009`._ OAuth relay completo (BYOA con browser redirect) es post-MVP.

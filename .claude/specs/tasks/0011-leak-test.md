---
task: "0011"
slug: leak-test
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: test
    name: leak-test
    path: tests/acceptance/0011__leak-test_test.go
scope:
  - tests/acceptance/0011__leak-test_test.go
  - scripts/leak-test.sh
---

# Task 0011 — `leak-test`

> **Acceptance gate del MVP.** Actuar como Usuario A (Org 1 / Grupo 1 / `group:member`) y afirmar que absolutamente ninguna row de Postgres ni objeto de Storage de Org 2, ni de Org 1/Grupo 2, es retornada. Si este test falla, el aislamiento se rompió.

> **Estado de dependencias (2026-05-31):** Task 0010 (`seed-fixture`) **mergeada** (PR #7) — `tests/helpers/fixture.go` se consume directamente. **R-STORAGE resuelto** (PR #9, migración `20260531000001_storage-layout-fix`, deployado a cloud): la capa de Storage del leak-test ya **no es vacua** — A ve sus propios objetos y debe NO ver los ajenos. Queda **un** riesgo abierto (R-HTTP-STUB, no bloqueante). Este test corre **verde** contra un Supabase vivo con el fixture sembrado.

## What

Dos entregables que consumen el fixture canónico de Task 0010 (`helpers.Seed` / `helpers.UserJWT`):

1. **`tests/acceptance/0011__leak-test_test.go`** — test Go de acceptance, build-tagged `//go:build acceptance`, que actúa como **Usuario A** (`helpers.UserIDA`: Org 1 / Grupo 1-general / `group:member`) y prueba cero fuga cross-tenant en **dos capas**. Hace `t.Skip` cuando faltan las env vars de Supabase vivo, para que `go test ./...` siga verde sin backend.
2. **`scripts/leak-test.sh`** — wrapper de CI (merge gate): corre `go test -tags=acceptance -run TestLeak ./tests/acceptance/... -v`; **exit 0** = aislamiento íntegro, **exit 1** = leak, imprimiendo `tabla/prefijo + id filtrado`.

### Actores del fixture (de `tests/helpers/fixture.go`, Task 0010)

| Constante | Quién | Visible para A? |
|---|---|---|
| `OrgID1` | Org de A | sí (su org) |
| `OrgID2` | otra org | **NO** |
| `GroupID1G1` | Org1 / Grupo 1 (`is_general=true`) — grupo de A | sí (su grupo) |
| `GroupID1G2` | Org1 / Grupo 2 (`is_general=false`) — grupo de B | **NO** (mismo org, otro grupo, no-general) |
| `GroupID2G1` | Org2 / Grupo 1 (general) | **NO** (otra org) |
| `AgentID1` | agente en Org1/G1 | sí |
| `AgentID2` | agente en Org2/G1 | **NO** |

`current_tenant()` resuelve `(OrgID1, GroupID1G1, group:member)` para A (vía `auth.uid()` del JWT). Como el grupo de A es general, sus grupos visibles = `{GroupID1G1}`.

### Capa 1 — Data-level (PostgREST bajo el JWT real de Usuario A; RLS en efecto, **nunca** service_role)

El cliente usa `apikey: <SUPABASE_ANON_KEY>` + `Authorization: Bearer <jwtA>`, donde `jwtA := helpers.UserJWT(t, url, serviceRoleKey, helpers.UserIDA)` (token real emitido por Supabase vía password-grant). Para cada tabla tenant-scoped se hace `GET /rest/v1/<tabla>?select=*` y se afirma la invariante de no-leak contra los datos realmente sembrados:

| Tabla | Target prohibido sembrado | Invariante para Usuario A |
|---|---|---|
| `organizations` | `OrgID2` | toda row `id == OrgID1`; `OrgID2` nunca aparece |
| `groups` | `GroupID1G2`, `GroupID2G1` | toda row es `GroupID1G1`; `GroupID1G2` y `GroupID2G1` ausentes |
| `memberships` | membership de B (G1G2) y de C (Org2) | A ve solo su propia membership; las de `UserIDB`/`UserIDC` ausentes |
| `agents` | `AgentID2` (Org2) | A ve solo `AgentID1`; `AgentID2` ausente |
| `runs` | *(fixture no siembra runs — ver canarios)* | A no ve ningún run de Org2 ni de Org1/G2 |
| `org_credentials` | *(fixture no siembra — ver canarios)* | A (member) ve **cero** rows |

**Canarios (arrange test-local con service_role, porque el fixture deja `runs` y `org_credentials` vacíos para los tenants prohibidos):** para que las aserciones de `runs` y `org_credentials` no sean vacuas, el test inserta vía service_role y limpia con `t.Cleanup`:
- un `runs` row cross-org: `agent_id=AgentID2, org_id=OrgID2, group_id=GroupID2G1, user_id=UserIDC, prompt='leak-canary', status='done'` → A no debe verlo.
- un `runs` row within-org Grupo 2: requiere primero un agente canario en `Org1/GroupID1G2` (sembrado y limpiado igual) → A no debe ver ni el agente ni el run.
- un `org_credentials` canario en `OrgID2` (y opcionalmente `OrgID1`) → A no debe ver ninguno.

**Storage:** listar objetos del bucket `houston` bajo el JWT de A (`POST /storage/v1/object/list/houston`, body `{"prefix":""}`). Con R-STORAGE resuelto, la aserción es **positiva + negativa**: A **sí** ve su objeto `<OrgID1>/<GroupID1G1>/agents/<AgentID1>/CLAUDE.md` (prueba que el aislamiento no es vacuo) y **no** ve ninguna key bajo `<OrgID2>/…` (objeto de `AgentID2`). El caso within-org Grupo 2 (el fixture no siembra objeto en `<OrgID1>/<GroupID1G2>/…`) se cubre opcionalmente con un canario Storage en ese prefijo (service_role + `t.Cleanup`).

### Capa 2 — HTTP-level (manejar el server real `/v1/*` como Usuario A)

Levantar `server.New(cfg, server.NewSupabaseQuerier(url, anon))` (con `cfg.JWTSecret = SUPABASE_JWT_SECRET` para que el auth middleware valide el token real de A) envuelto en `httptest.NewServer`, requests con `Authorization: Bearer <jwtA>`:

- `GET /v1/agents/{AgentID2}` (agente de Org2) → NO debe retornar la row (404/403; el body nunca contiene campos del agente ajeno). *(Ruta hoy en `stubHandler` → se ejerce failing-first con `t.Skip("pending: get-agent handler no implementado")`.)*
- Cualquier endpoint de listado que exista → el body no contiene ningún id de entidad ajena.
- **Por qué la Capa 2:** aun con RLS correcto, un handler que usara la service_role key sobre-traería datos. Esto atrapa esa clase de bug en la superficie que un cliente real golpea.

**Resultado agregado:** cualquier leak único en cualquier capa → exit ≠ 0 + impresión de `tabla/prefijo + id filtrado`.

## Why

La invariante norte del PRD/RFC: **el aislamiento multi-tenant se garantiza en la capa de datos (Postgres + Storage RLS), no solo en código de aplicación** (RFC §1, §4.3). Este test es la prueba automatizada de esa invariante y el merge gate del MVP (RFC §6 R1: "si una policy está mal escrita, el leak test falla. Gate automático.").

Dos capas porque atrapan modos de falla distintos:

- **Data-level** verifica las policies RLS mismas (`20260530000001_rls-postgres.sql`, `current_tenant()`). Una regresión en una policy aparece acá.
- **HTTP-level** verifica que la aplicación nunca esquive RLS (p. ej. un handler usando service_role, o una query que cruce el límite tenant). La capa de datos puede ser perfecta mientras un handler igual filtra.

Un leak-test verde es la señal más importante de que el control plane multi-tenant es seguro para shippear.

## How

1. **Build tag** `//go:build acceptance` (excluido de `go test ./...` y de `-tags=integration`; corre solo vía el script gate o `-tags=acceptance`).
2. **Skip guard**: leer `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_ANON_KEY`, `SUPABASE_JWT_SECRET`; si falta `SUPABASE_URL` o `SUPABASE_SERVICE_ROLE_KEY` → `t.Skip(...)` (patrón espejo de `tests/integration/0010__seed-fixture_test.go`). La Capa 2 además requiere `ANON_KEY` + `JWT_SECRET`.
3. **Fixture**: `ids := helpers.Seed(t, url, serviceRoleKey)` — el teardown se registra automáticamente vía `t.Cleanup` (no hace falta `defer`).
4. **JWT de A**: `jwtA := helpers.UserJWT(t, url, serviceRoleKey, ids.UserIDA)` (password-grant → access_token real con `sub=UserIDA`, validado por PostgREST y por el auth middleware con el JWT secret del proyecto).
5. **Data-level**: `t.Run(tabla, …)` table-driven emitiendo GETs a `/rest/v1/<tabla>` con el cliente de A; decodificar arrays JSON; afirmar la invariante por tabla contra `ids`.
6. **Canarios**: helper local que inserta los runs/agent/credentials canarios vía service_role (mismo patrón HTTP que `helpers.upsertRow`) y los borra en `t.Cleanup`.
7. **Storage-level**: `POST /storage/v1/object/list/houston` con el JWT de A; afirmar ausencia de keys de tenants prohibidos.
8. **HTTP-level**: `srv := httptest.NewServer(server.New(cfg, server.NewSupabaseQuerier(url, anon)))`; requests `/v1/*` con el bearer de A; `t.Skip` con motivo para rutas en stub.
9. **`scripts/leak-test.sh`**: `set -euo pipefail`; cargar `.env` si existe; `go test -tags=acceptance -run TestLeak ./tests/acceptance/... -v`; propagar exit code.
10. **Header de mapeo AC→test** en comentario (espejo de la convención de `tests/integration/0007__create-agent-flow_test.go` y `0010`).

## Fixture API consumida (Task 0010 — MERGEADA, PR #7)

`import "github.com/nomenclator/houston2/tests/helpers"`. API real disponible (ya no es un contrato asumido):

```go
// Constantes UUID determinísticas a nivel de paquete:
helpers.OrgID1, helpers.OrgID2,
helpers.GroupID1G1, helpers.GroupID1G2, helpers.GroupID2G1,
helpers.UserIDA, helpers.UserIDB, helpers.UserIDC,
helpers.AgentID1, helpers.AgentID2 // uuid.UUID

type FixtureIDs struct { // todos los campos == las constantes de arriba
    OrgID1, OrgID2,
    GroupID1G1, GroupID1G2, GroupID2G1,
    UserIDA, UserIDB, UserIDC,
    AgentID1, AgentID2 uuid.UUID
}

// Siembra el fixture vía service_role y registra t.Cleanup(Teardown).
func Seed(t testing.TB, supabaseURL, serviceRoleKey string) FixtureIDs
// Cleanup idempotente (también llamado automáticamente por Seed).
func Teardown(supabaseURL, serviceRoleKey string) error
// access_token real de Supabase (password-grant) para un usuario fixture; sub=userID.
func UserJWT(t testing.TB, supabaseURL, serviceRoleKey string, userID uuid.UUID) string
```

## Riesgos cross-task (descubiertos al reconciliar con la infra real)

- **R-STORAGE — ✅ RESUELTO (PR #9, mergeado + deployado a cloud).** La policy `houston_tenant_isolation` exigía `(storage.foldername(name))[1] = 'houston'` pero los objetos se nombran relativos al bucket (`<org>/<group>/...`). Se corrigió la policy (`[1]=org`, `[2]=group`, general vía `is_general`) en `20260531000001_storage-layout-fix.sql`. Ahora un usuario autenticado **sí** ve sus objetos → la capa de Storage del leak-test es **real** (no vacua). Sin acción pendiente en 0011.
- **R-HTTP-STUB.** `GET /v1/agents/{id}` está hoy en `stubHandler` (no hay handler get-agent). La aserción HTTP de no-leak se autoriza failing-first con `t.Skip` motivado hasta que esa ruta exista.
- **R-FIXTURE-CANARIOS.** El fixture (por diseño, 0010 §out-of-scope) no siembra `runs` ni `org_credentials`. 0011 arregla canarios test-local (service_role + `t.Cleanup`) para que esas aserciones no sean vacuas.

## Acceptance criteria

1. **AC1 — organizations no-leak**: como Usuario A, `GET /rest/v1/organizations` retorna solo `OrgID1`; `OrgID2` nunca aparece.
2. **AC2 — groups no-leak**: A ve solo `GroupID1G1`; `GroupID1G2` y `GroupID2G1` ausentes.
3. **AC3 — memberships no-leak**: A ve solo su propia membership; las de `UserIDB` (Org1/G2) y `UserIDC` (Org2) ausentes.
4. **AC4 — agents no-leak**: A ve solo `AgentID1`; `AgentID2` (Org2) ausente.
5. **AC5 — runs no-leak (con canarios)**: con runs canarios sembrados en Org2 y en Org1/Grupo 2 (vía service_role), A no ve ninguno.
6. **AC6 — credentials denied**: con un `org_credentials` canario en Org2 (y/o Org1), `GET /rest/v1/org_credentials` retorna **cero** rows para el member A.
7. **AC7 — Storage aislamiento (positivo + negativo)**: bajo el JWT de A, el listado del bucket `houston` **incluye** su objeto `<OrgID1>/<GroupID1G1>/…/<AgentID1>/CLAUDE.md` y **no incluye** ninguna key de `<OrgID2>/…`. (R-STORAGE resuelto → no vacua.)
8. **AC8 — HTTP no-leak (agents)**: `GET /v1/agents/{AgentID2}` nunca retorna la row (404/403). *(failing-first: `t.Skip` motivado mientras la ruta sea stub — R-HTTP-STUB.)*
9. **AC9 — usa el JWT del caller, no service_role**: todos los requests de probe usan `apikey: anon` + `Bearer <jwtA>`; el service_role solo se usa para arrange/cleanup de canarios, nunca para las aserciones de no-leak.
10. **AC10 — semántica de gate CI**: `scripts/leak-test.sh` sale 0 con integridad total, 1 con `tabla/prefijo + id` ofensor ante cualquier leak.
11. **AC11 — verde sin backend**: con las env vars de Supabase sin setear, la suite hace `t.Skip` (no fail), manteniendo `go test ./...` verde.
12. **AC12 — corre contra infra real**: con `SUPABASE_URL` + `SUPABASE_SERVICE_ROLE_KEY` + `SUPABASE_ANON_KEY` + `SUPABASE_JWT_SECRET` válidos y el fixture de 0010, la suite ejecuta las dos capas y reporta pass/fail por tabla/prefijo.

## Out of scope

- Aserciones de enforcement RBAC 403 → **Task 0012**.
- Construir/extender el fixture base → **Task 0010** (Mauro). Los canarios de `runs`/`org_credentials`/agente-G1G2 son arrange test-local de 0011, no del fixture.
- Arreglar el mismatch de naming/policy de Storage (R-STORAGE) → **Task 0003** (reconciliación separada).
- Implementar el handler get-agent / run-agent → **Tasks 0008 / futuras**.
- Usuarios multi-grupo (`LIMIT 1` en `current_tenant`) → post-MVP (RFC §6 R2).

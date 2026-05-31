---
task: "0012"
slug: rbac-test
granularity: slice
version: 0.1.0
status: skeleton
declares:
  - type: test
    name: rbac-test
    path: tests/acceptance/0012__rbac-test_test.go
scope:
  - tests/acceptance/0012__rbac-test_test.go
  - scripts/rbac-test.sh
---

# Task 0012 — `rbac-test`

> **Acceptance gate del MVP.** Verifica que cada operación fuera del rol permitido retorna exactamente **403** y cada operación dentro de rol NO retorna 403. Cubre la matriz completa rol × operación de RFC §4.5.

> **Estado de dependencias (2026-05-31, actualizado):** consume el fixture de Task 0010 (PR #7) — 3 actores: **A** (`group:member`), **B** (`group:manager`), **C** (`org:owner`). **Routing real (`internal/server/server.go`):** ruteados y enforced hoy: `POST /v1/agents` (RoleManager, 0007) **y `POST /v1/agents/{id}/runs` (RoleMember, Task 0008 ya mergeada — PR #12)**. Siguen **ausentes** (404): groups/memberships/credentials CRUD; **stub**: `GET /v1/runs/{id}`. Auth ahora valida tokens **reales ES256/JWKS** (fix #13), así los `helpers.UserJWT` reales pasan el middleware. **Celdas live:** crear-agente (role gate member→403, manager/owner→201), run-agent (denegaciones **cross-org y cross-grupo** → **404** vía RLS; el cross-grupo con un agente canario en Org1/Group2), y auth (sin JWT → 401). El resto, skip-pending — el test autorea la matriz completa igual.

## What

Dos entregables que consumen el fixture de Task 0010 (`helpers.Seed` / `helpers.UserJWT`):

1. **`tests/acceptance/0012__rbac-test_test.go`** — test Go de acceptance, build-tag `//go:build acceptance`, **table-driven** sobre la matriz completa de RFC §4.5. Cada fila = `{actor, método, path, body, esperado}`. Para toda combinación **prohibida** afirma exactamente **403** con `error.code == "forbidden"`; para toda combinación **permitida** afirma **NO 403** (2xx). Filas cuyo endpoint aún no existe → `t.Skip("pending <task>: <endpoint> no ruteado")`. `t.Skip` sin las env vars de Supabase vivo.
2. **`scripts/rbac-test.sh`** — wrapper de CI (merge gate), simétrico a `scripts/leak-test.sh`: corre `go test -tags=acceptance -run TestRBAC ./tests/acceptance/... -v`; exit 0 = RBAC correcto, 1 = violación, 2 = sin env. **[Adición de scope vs skeleton — flagged para el gate.]**

### Actores (de `tests/helpers/fixture.go`)

| Actor | Rol | Org / Grupo | JWT |
|---|---|---|---|
| **A** `UserIDA` | `group:member` | Org1 / Group1-general | `helpers.UserJWT(t,…,UserIDA)` |
| **B** `UserIDB` | `group:manager` | Org1 / Group2 (sin agente sembrado) | `…UserIDB` |
| **C** `UserIDC` | `org:owner` | Org2 / Group1-general | `…UserIDC` |

Agentes sembrados: `AgentID1` en Org1/Group1, `AgentID2` en Org2/Group1. **No hay agente en Org1/Group2** (grupo de B) → el caso "manager opera sobre agente de otro grupo" usa `AgentID1` con el JWT de B.

### Matriz RFC §4.5 (fuente de verdad) — con estado de routing

Enforcement: middleware Go `RequireRole(minRole)` retorna **403** `{"error":{"code":"forbidden",…}}` si `tc.Role < minRole` (`internal/middleware/rbac.go`, `errors.go`). Cadena: `Auth(401) → Tenant(403 sin membership) → RequireRole(403) → handler`.

| # | Actor | Operación | Endpoint | Esperado | Estado |
|---|---|---|---|---|---|
| M1 | member A | crear agente | `POST /v1/agents` | **403** forbidden | ✅ **live** |
| M2 | manager B | crear agente (su grupo) | `POST /v1/agents` | permitido (201) | ✅ **live** |
| M3 | owner C | crear agente | `POST /v1/agents` | permitido (201) | ✅ **live** |
| M4 | (sin JWT / JWT inválido) | crear agente | `POST /v1/agents` | **401** unauthenticated | ✅ **live** (gate de auth) |
| M5 | member A | CRUD grupos | `POST/PUT/DELETE /v1/groups…` | 403 | ⏸ pending (ruta ausente → 404) |
| M6 | manager B | CRUD grupos | `…/v1/groups…` | 403 | ⏸ pending |
| M7 | owner C | CRUD grupos | `…/v1/groups…` | permitido | ⏸ pending |
| M8 | member A | gestionar membership | `…/v1/memberships…` | 403 | ⏸ pending |
| M9 | manager B | membership de su grupo | `…/v1/memberships…` | permitido | ⏸ pending |
| M10 | manager B | membership de otro grupo | `…/v1/memberships…` | 403 | ⏸ pending |
| M11 | owner C | cualquier membership | `…/v1/memberships…` | permitido | ⏸ pending |
| M12 | member A | gestionar credenciales | `POST/PUT/DELETE/GET /v1/orgs/{id}/credentials` | 403 | ⏸ pending (handlers existen, sin rutear → 404) |
| M13 | manager B | gestionar credenciales | `…/credentials` | 403 | ⏸ pending |
| M14 | owner C | credenciales de su org | `…/credentials` | permitido | ⏸ pending |
| M15 | member A | correr agente propio (Org1/general) | `POST /v1/agents/{AgentID1}/runs` | permitido (no 403/404) | ✅ **live** (funcional; dispara claude REAL → no se asserta en el gate, ver nota) |
| M16 | member A (Org1) | correr agente de **otra org** | `POST /v1/agents/{AgentID2}/runs` | **404** (RLS: no visible) | ✅ **live** |
| M17 | member A (Org1/G1) | correr **agente canario en Org1/Group2** (otro grupo, mismo org, no-general) | `POST /v1/agents/{canaryG2}/runs` | **404** (RLS: grupo no visible) | ✅ **live** (con agente canario) |
| M18 | cualquiera | leer run ajeno | `GET /v1/runs/{id}` | 403/404 | ⏸ pending (stub, sin RBAC) |
| M19 | owner C (Org2) | credenciales de **otra org** | `POST /v1/orgs/{Org1}/credentials` | 403/404 (cross-org) | ⏸ pending (sin rutear; el handler ya valida `{id}==tc.OrgID`→403) |

Nota M2/M3: `POST /v1/agents` crea el agente en el **propio** tenant del caller (org/grupo derivados del token); no hay parámetro de grupo, así que el matiz "cualquier grupo del org" del owner no es expresable en este endpoint (se cubrirá cuando existan rutas con `{group_id}`/`{agent_id}`).

**Nota run-agent (M15–M17):** el piso de `POST /v1/agents/{id}/runs` es `RoleMember` → **no hay 403 por rol** (todo miembro corre los agentes que *ve*). El control es **RLS**: `GetAgent` con el JWT del caller solo retorna agentes de sus grupos visibles → un agente de **otra org** da **404** (no 403). El caso *permitido* (M15, correr el agente propio) dispara una corrida **REAL de claude** (~7s + costo de tokens) → **no se asserta en el gate** RBAC (es funcional, cubierto por 0008); el gate asserta las **denegaciones** (404 cross-org + 401 sin JWT), baratas porque fallan en `GetAgent`/auth **antes** de correr claude. El cross-**grupo dentro del mismo org SÍ se testea**, pero requiere **arreglar un agente canario en `Org1/Group2`** (grupo no-general de B) vía service_role — el fixture solo siembra agentes en grupos *general* (visibles a todos), así que sin canario no hay caso de denegación cross-grupo. Con el canario: A (miembro de Group1-general) corre ese agente → **404** (A no ve Group2). Patrón idéntico a los canarios de 0011. El cross-grupo es **parte esencial** del aislamiento del MVP y la RLS de `agents` (0002) ya lo enforce; este test lo verifica a nivel HTTP.

## Why

- **RFC: el MVP debe "demostrar RBAC con 3 roles".** Este test es la prueba automatizada de que el enforcement no es solo código de middleware sino comportamiento observable (403 real).
- **Anti-regresión:** si un PR futuro quita un wrapper `RequireRole` o baja el rol mínimo de un endpoint, esta gate lo caza.
- **Scaffold que se enciende:** igual que el leak-test de 0011 encendió su capa de Storage al resolverse R-STORAGE, esta matriz se vuelve verde célula por célula a medida que se ruteen groups/memberships/credentials/run-agent. Documenta exhaustivamente el contrato RBAC esperado.

## How

1. **Build tag** `//go:build acceptance` (excluido de `go test ./...`).
2. **Skip guard**: leer `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_ANON_KEY`, `SUPABASE_JWT_SECRET`; si falta `SUPABASE_URL`/`SERVICE_ROLE_KEY` → `t.Skip`. (Auth + querier necesitan `ANON_KEY` + `JWT_SECRET`.)
3. **Fixture**: `ids := helpers.Seed(t, url, serviceRoleKey)` (teardown automático). JWTs: `jwtA/jwtB/jwtC := helpers.UserJWT(t, url, serviceRoleKey, ids.UserID{A,B,C})`.
4. **Server real**: `srv := server.New(server.Config{SupabaseURL:url, AnonKey:anon, JWTSecret:jwtSecret, Port:"0"}, server.NewSupabaseQuerier(url, anon))`. Se invoca con `srv.ServeHTTP(httptest.NewRecorder(), req)` — el `supabaseQuerier` deriva `(org,group,role)` llamando a `current_tenant()` con el JWT real del actor (sin red extra).
5. **Table-driven**: por cada fila de la matriz, `t.Run(nombre, …)`: construir el request con `Authorization: Bearer <actorJWT>`; afirmar el status esperado y, para 403, decodificar el envelope y verificar `error.code == "forbidden"`.
6. **Filas pending**: `t.Skip("pending <task>: <endpoint> no ruteado")` — la suite es verde-cuando-aplica y auto-documenta los gaps de cobertura.
7. **Cleanup de side-effects**: las filas permitidas que pegan `POST /v1/agents` (M2/M3) crean un agente real (fila en `agents` + objeto en Storage). El test captura el `id` de la respuesta 201 y registra `t.Cleanup` para borrarlo vía service_role (`DELETE /rest/v1/agents?id=eq.<id>` + el objeto `houston/<org>/<group>/agents/<id>/CLAUDE.md`).
8. **`scripts/rbac-test.sh`**: `set -uo pipefail`; exige env; `go test -tags=acceptance -run TestRBAC ./tests/acceptance/... -v`; propaga exit code.
9. **Header de mapeo AC→test** (espejo de `0011`/`0007`).

## Fixture API consumida (Task 0010 — mergeada, PR #7)

```go
helpers.UserIDA, helpers.UserIDB, helpers.UserIDC,
helpers.OrgID1, helpers.OrgID2, helpers.GroupID1G1, helpers.GroupID1G2, helpers.GroupID2G1,
helpers.AgentID1, helpers.AgentID2 // uuid.UUID
func Seed(t testing.TB, supabaseURL, serviceRoleKey string) FixtureIDs   // teardown vía t.Cleanup
func UserJWT(t testing.TB, supabaseURL, serviceRoleKey string, userID uuid.UUID) string // token real
```

## Riesgos / dependencias

- **R-RUTAS-PENDIENTES.** La mayoría de la matriz (M5–M19) pega endpoints **no ruteados** (404) o **stub** (sin RBAC). Esas celdas se autorean failing-first con `t.Skip` motivado, referenciando la task/PR que las desbloquea (groups/memberships/credentials-wiring/run-agent). Live hoy: M1–M4 (`POST /v1/agents` + auth).
- **Dependencia de wiring de credenciales (0009 diferido).** M12–M14, M18 dependen de rutear `/v1/orgs/{id}/credentials` (handlers ya existen; el wiring lo difirió 0009 a propósito — no es bug). Skip-pending hasta entonces.
- **Side-effects + canario.** M2/M3 escriben agentes reales → cleanup con service_role + `t.Cleanup`. Para el cross-grupo (M17) el test **arregla un agente canario en `Org1/Group2`** (id fijo, vía service_role) y lo borra en `t.Cleanup` (LIFO, antes del teardown del fixture).

## Acceptance criteria

1. **AC1 — member→crear-agente prohibido (live)**: `POST /v1/agents` con el JWT de A (member) → **403** y `error.code=="forbidden"`; el handler no se ejecuta.
2. **AC2 — manager→crear-agente permitido (live)**: `POST /v1/agents` con el JWT de B (manager), `{name,source:"blank"}` → status **NO 403** (201). El agente creado se limpia.
3. **AC3 — owner→crear-agente permitido (live)**: idem con el JWT de C (owner) → **NO 403** (201). Cleanup.
4. **AC4 — auth gate (live)**: `POST /v1/agents` sin `Authorization` (o JWT inválido) → **401** (no 403, no 201).
5. **AC5 — envelope 403**: toda respuesta 403 de la suite cumple el formato RFC §4.11 `{"error":{"code":"forbidden","message":<no vacío>,…}}`.
6. **AC6 — matriz completa autorada**: existe una fila de test por cada celda M1–M19; las no-live emiten `t.Skip` con motivo que nombra la task/endpoint pendiente (cobertura auto-documentada, sin falsos verdes silenciosos).
7. **AC7 — semántica de gate CI**: `scripts/rbac-test.sh` sale 0 si todo OK, 1 ante cualquier violación de RBAC, 2 sin env.
8. **AC8 — verde sin backend**: sin las env vars, la suite hace `t.Skip` (no fail); `go test ./...` (sin tag) sigue verde.
9. **AC9 — corre contra infra real**: con env válidas + fixture de 0010, la suite ejecuta las celdas live y reporta pass/fail por celda.
10. **AC10 — sin side-effects persistentes**: tras la corrida, los agentes creados por M2/M3 fueron borrados (la fixture queda en su estado canónico).
11. **AC11 — run-agent: denegaciones (live)**: **cross-org** — `POST /v1/agents/{AgentID2}/runs` con el JWT de A (Org1) → **404**; **cross-grupo** — con un agente canario sembrado en `Org1/Group2`, `POST /v1/agents/{canaryG2}/runs` con el JWT de A (Group1-general) → **404**; **auth** — sin `Authorization` → **401**. El caso *permitido* (A corre su propio `AgentID1`) NO se asserta (dispara claude real — ver Nota run-agent). Todas las denegaciones fallan en `GetAgent` antes de cualquier subprocess → sin costo.

## Out of scope

- Aislamiento de datos / no-leak de rows y Storage → **Task 0011** (ya mergeada).
- **Construir** los endpoints aún no ruteados (groups/memberships CRUD, wiring de credenciales) — esta task solo **afirma** su comportamiento RBAC; el wiring es de sus tasks (0006/0009/futuras). *(run-agent ya está ruteado por 0008.)*
- Construir/extender el fixture base → **Task 0010** (Mauro).
- Usuarios multi-grupo (`LIMIT 1` en `current_tenant`) → post-MVP (RFC §6 R2).

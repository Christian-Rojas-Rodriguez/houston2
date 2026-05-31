---
task: "0010"
slug: seed-fixture
granularity: slice
version: 0.1.1
status: ready
declares:
  - type: script
    name: seed-fixture
    path: scripts/seed.go
  - type: sql
    name: seed-fixture-sql
    path: tests/testdata/seed.sql
  - type: helper
    name: fixture-helper
    path: tests/helpers/fixture.go
scope:
  - scripts/seed.go
  - tests/testdata/seed.sql
  - tests/helpers/fixture.go
---

# Task 0010 — `seed-fixture`

> Script reproducible que crea el fixture MVP: 2 orgs / 3 grupos / 3 usuarios / 3 roles / 2 agents. Usado por los acceptance tests (Tasks 0011 y 0012) para tener un estado de partida determinista. Requiere `SUPABASE_URL` + `SUPABASE_SERVICE_ROLE_KEY`. Usa UUIDs fijos hardcodeados.

## What

Produce tres artefactos que en conjunto constituyen el fixture canónico del MVP:

### 1. `scripts/seed.go`

Script Go ejecutable con `go run scripts/seed.go`. Inserta el fixture en Supabase cloud usando el `service_role` key (bypasea RLS para setup). Completamente idempotente: correrlo dos veces produce el mismo estado sin errores de clave duplicada.

**Entidades que inserta:**

| Entidad | Detalle |
|---|---|
| Org 1 | UUID determinístico `OrgID1` |
| Org 2 | UUID determinístico `OrgID2` |
| Grupo Org1/G1 | `is_general=true`, UUID `GroupID1G1` |
| Grupo Org1/G2 | `is_general=false`, UUID `GroupID1G2` |
| Grupo Org2/G1 | `is_general=true`, UUID `GroupID2G1` |
| Usuario A | email `user-a@houston-fixture.test`, UUID `UserIDA` |
| Usuario B | email `user-b@houston-fixture.test`, UUID `UserIDB` |
| Usuario C | email `user-c@houston-fixture.test`, UUID `UserIDC` |
| Membership A | A → Org1/G1 / `group:member` |
| Membership B | B → Org1/G2 / `group:manager` |
| Membership C | C → Org2/G1 / `org:owner` |
| Agent 1 | `source=blank`, en Org1/G1, UUID `AgentID1` |
| Agent 2 | `source=blank`, en Org2/G1, UUID `AgentID2` |
| Storage Agent1 | `houston/{OrgID1}/{GroupID1G1}/agents/{AgentID1}/CLAUDE.md` |
| Storage Agent2 | `houston/{OrgID2}/{GroupID2G1}/agents/{AgentID2}/CLAUDE.md` |

El flag `-teardown` elimina todas las entidades en orden inverso de dependencias.

### 2. `tests/testdata/seed.sql`

Companion SQL idempotente que produce el mismo estado de fixture usando `service_role` context (sin RLS). Alternativa para entornos donde el runtime Go no está disponible o para ejecución directa en Supabase SQL editor. Usa `INSERT ... ON CONFLICT DO NOTHING` con los mismos UUIDs determinísticos.

### 3. `tests/helpers/fixture.go`

Paquete Go `package helpers` que envuelve la lógica del script para uso en acceptance tests. Exporta:

```go
// FixtureIDs contiene todos los UUIDs determinísticos del fixture MVP.
type FixtureIDs struct {
    OrgID1     uuid.UUID
    OrgID2     uuid.UUID
    GroupID1G1 uuid.UUID // Org1 / Grupo 1 (general)
    GroupID1G2 uuid.UUID // Org1 / Grupo 2
    GroupID2G1 uuid.UUID // Org2 / Grupo 1 (general)
    UserIDA    uuid.UUID // group:member en Org1/G1
    UserIDB    uuid.UUID // group:manager en Org1/G2
    UserIDC    uuid.UUID // org:owner en Org2/G1
    AgentID1   uuid.UUID // agente en Org1/G1
    AgentID2   uuid.UUID // agente en Org2/G1
}

// Seed inserta el fixture en Supabase usando service_role.
// Registra t.Cleanup para llamar a Teardown automáticamente.
// Falla el test con t.Fatal si cualquier step de seed falla.
func Seed(t testing.TB, supabaseURL, serviceRoleKey string) FixtureIDs

// Teardown elimina todas las entidades del fixture en orden inverso de dependencias.
// Idempotente: no falla si alguna entidad ya no existe.
func Teardown(supabaseURL, serviceRoleKey string) error

// UserJWT retorna un JWT válido de Supabase para el usuario fixture dado.
// Usa la Auth Admin API para generar un magic link y extrae el token.
// Falla el test con t.Fatal si no puede obtener el token.
func UserJWT(t testing.TB, supabaseURL, serviceRoleKey string, userID uuid.UUID) string
```

## Why

Tasks 0011 (`leak-test`) y 0012 (`rbac-test`) son los acceptance gates del MVP completo. Ambas necesitan un estado conocido y determinista en Supabase para hacer assertions válidas.

- **UUIDs determinísticos**: sin UUIDs fijos, las assertions cross-test (`org_id no debe aparecer en resultados para Usuario A`) son frágiles y no reproducibles.
- **Teardown automático via `t.Cleanup`**: sin cleanup, los tests dejan estado que contamina ejecuciones posteriores y en CI produce falsos negativos.
- **service_role para setup**: las políticas RLS bloquearían los inserts si se usara un JWT de usuario normal. El seed tiene que bypasear RLS exactamente igual que lo hacen las migraciones.
- **Cloud Supabase real**: los acceptance tests validan RLS real en infra real, no mocks. La fixture también debe poder ejecutarse contra cloud.

## How

### Patrones de implementación

**Dependencias Go:** solo stdlib + `github.com/google/uuid` (ya en `go.mod`). Ninguna nueva dependencia.

**HTTP pattern:** igual que `scripts/seed-templates.sh` y `internal/handlers/supabase_agent_store.go`:
- Todas las llamadas van a la REST API de Supabase (`/rest/v1/` para tablas, `/storage/v1/` para objetos).
- Usuarios se crean via Auth Admin API: `POST /auth/v1/admin/users` con header `Authorization: Bearer <service_role>`.
- Header `Prefer: resolution=ignore-duplicates` para idempotencia en inserts de tablas.
- Header `x-upsert: true` para idempotencia en uploads de Storage.

**UUIDs determinísticos:** todos los IDs se declaran como constantes `uuid.MustParse(...)` al inicio del archivo. Los valores son fijos y forman parte del contrato de la spec (no cambiar sin bump de version).

```go
var (
    OrgID1     = uuid.MustParse("00000010-0000-0000-0000-000000000001")
    OrgID2     = uuid.MustParse("00000010-0000-0000-0000-000000000002")
    GroupID1G1 = uuid.MustParse("00000010-0000-0001-0000-000000000001")
    GroupID1G2 = uuid.MustParse("00000010-0000-0001-0000-000000000002")
    GroupID2G1 = uuid.MustParse("00000010-0000-0002-0000-000000000001")
    UserIDA    = uuid.MustParse("00000010-0000-0000-0001-000000000001")
    UserIDB    = uuid.MustParse("00000010-0000-0000-0001-000000000002")
    UserIDC    = uuid.MustParse("00000010-0000-0000-0001-000000000003")
    AgentID1   = uuid.MustParse("00000010-0000-0000-0002-000000000001")
    AgentID2   = uuid.MustParse("00000010-0000-0000-0002-000000000002")
)
```

**Orden de seed (dependencias):**
1. Auth users (sin FKs)
2. Organizations (sin FKs)
3. Groups (FK → organizations)
4. Memberships (FK → auth.users, organizations, groups)
5. Agents (FK → organizations, groups)
6. Storage objects (referencia org_id + group_id + agent_id)

**Orden de teardown (inverso):**
1. Storage objects
2. Agents
3. Memberships
4. Groups
5. Organizations
6. Auth users

**env vars:**
```
SUPABASE_URL=https://<project-ref>.supabase.co
SUPABASE_SERVICE_ROLE_KEY=<service_role_key>
```

**Emails de usuarios fixture:**
- `user-a@houston-fixture.test` (password fija: `houston-fixture-pass-A`)
- `user-b@houston-fixture.test` (password fija: `houston-fixture-pass-B`)
- `user-c@houston-fixture.test` (password fija: `houston-fixture-pass-C`)

**`UserJWT` en `fixture.go`:** llama a `POST /auth/v1/token?grant_type=password` con las credenciales del usuario fixture para obtener un `access_token` válido. Este JWT puede usarse en los acceptance tests para hacer requests autenticados que pasen por RLS.

**`tests/testdata/seed.sql`:** usa `INSERT INTO ... ON CONFLICT DO NOTHING` con los mismos UUIDs. No puede insertar en `auth.users` directamente via SQL en cloud Supabase (requiere Auth Admin API); el SQL de usuarios es solo documentación/referencia para local dev con `auth.users` mocked.

## Acceptance criteria

- **AC1** — `go run scripts/seed.go` con `SUPABASE_URL` y `SUPABASE_SERVICE_ROLE_KEY` válidos termina con exit 0 e imprime confirmación de cada recurso insertado (org, group, user, membership, agent, storage object).
- **AC2** — Correr `go run scripts/seed.go` una segunda vez termina con exit 0 sin errores de clave duplicada (idempotencia).
- **AC3** — `go run scripts/seed.go -teardown` termina con exit 0 y elimina todas las entidades del fixture en orden inverso de dependencias. Una consulta posterior con service_role a `organizations` no retorna las orgs del fixture.
- **AC4** — Después del seed, una query `GET /rest/v1/organizations` con el service_role key retorna al menos las 2 orgs del fixture identificadas por `OrgID1` y `OrgID2`.
- **AC5** — Después del seed, `GET /rest/v1/memberships` con service_role retorna exactamente 3 membership rows para los user IDs del fixture, con roles correctos (`group:member`, `group:manager`, `org:owner`).
- **AC6** — `helpers.Seed(t, url, key)` retorna un `FixtureIDs` con todos los campos no-zero, cuyos valores coinciden con las constantes UUID del paquete.
- **AC7** — `helpers.Seed(t, url, key)` registra un `t.Cleanup` que llama a `Teardown` — tras el test, las entidades del fixture no existen en Supabase.
- **AC8** — `helpers.UserJWT(t, url, key, UserIDA)` retorna un string no vacío que, al ser decodificado como JWT, tiene `sub == UserIDA.String()`.
- **AC9** — `helpers.UserJWT` con un userID que no existe en Supabase llama a `t.Fatal` (no retorna un token inválido silenciosamente).
- **AC10** — `tests/testdata/seed.sql` ejecutado con service_role en un Supabase local (vía `supabase db execute`) produce el mismo estado de tablas que el script Go (mismos UUIDs, mismas filas — excepto auth.users que requiere la Admin API).

## Out of scope

- Usuarios autenticados con Google OAuth real — los usuarios fixture usan email/password exclusivamente (para uso en tests).
- Objetos Storage en `templates/` — eso es responsabilidad de Task 0003 (`seed-templates.sh`). Esta task solo crea objetos bajo `houston/`.
- Runs del fixture — no se insertan filas en la tabla `runs`.
- Memberships múltiples por usuario — MVP: un usuario tiene exactamente una membership.
- Seeding de `org_credentials` — la API key de Anthropic es responsabilidad de Task 0009 y se configura por separado.
- Tests de acceptance de leak/RBAC — esos van en Tasks 0011 y 0012 respectivamente, que consumen este helper.

---
version: 0.1.0
status: draft
prd: .claude/specs/prd.md
---

# RFC: Houston 2.0 — Diseño técnico del MVP

| Campo | Valor |
|---|---|
| **Autor(es)** | Christian Rojas |
| **Revisores** | — |
| **PRD relacionado** | [.claude/specs/prd.md](.claude/specs/prd.md) |

## §1. Resumen

Este RFC describe la arquitectura técnica del MVP de Houston 2.0: un orquestador Go + Supabase (Postgres RLS + Storage RLS) + Claude Code local. El aislamiento multi-tenant se garantiza en la capa de datos (RLS), no solo en la aplicación. El MVP prueba la invariante de aislamiento con un leak test automatizado y demuestra RBAC con 3 roles (`org:owner`, `group:manager`, `group:member`).

## §2. Contexto y motivación

Houston (referencia) es single-tenant y local-first: sus 15 engine crates no tienen `org_id`/tenant, y sus módulos `teams/` y `cloud/` son stubs. El control plane multi-tenant es nuestro para construir — pero como una capa encima de Houston, no un fork. Ver PRD §2.

## §3. Goals / Non-goals

**Goals:**
- Definir el modelo de datos, RLS, RBAC y los flujos de create-agent y run-agent.
- Especificar la estrategia de concurrencia (workspace aislado por run).
- Fijar la granularidad de Tasks (§7) y la declaración POA (§8) del workflow de desarrollo.

**Non-goals:**
- La implementación interna de cada task — eso vive en cada Task spec.
- WebSocket / streaming — HTTP-only en MVP.
- Cloud hosting, RAG, UI — post-MVP.

## §4. Diseño propuesto

### 4.1 Arquitectura general

```
┌──────────────────────────────────────────────────┐
│  CONTROL PLANE  (construimos)                     │
│  Orquestador Go · identity→org/group/role         │
│  Postgres+Storage RLS · hydrate→run→sync          │
│  Supabase project PROPIO (nuevo, no el de Houston)│
│  Auth: PKCE + Google SSO (patrón de Houston)      │
│  Leak test + RBAC test                            │
├──────────────────────────────────────────────────┤
│  FORMAT + RUNTIME  (reusamos de Houston)          │
│  houston.json / CLAUDE.md / .houston/ layout      │
│  seed_agent · build_agent_context (in-instance)   │
│  BYOA relay: lógica reimplementada en Go          │
│  (login_relay.rs es library Rust, no invocable)   │
└──────────────────────────────────────────────────┘
```

**Stack decidido:**
- **Orquestador:** Go (binario único, manejo nativo de subprocesos concurrentes, type-safe)
- **Base de datos:** Supabase Postgres con RLS habilitado en todas las tablas tenant-scoped
- **Storage:** Supabase Storage con RLS por prefix de bucket
- **Runtime:** Claude Code CLI como subprocess local (MVP)
- **Transport:** HTTP solamente, rutas `/v1/*` mirroring Houston

### 4.2 Modelo de datos

**Tablas Postgres:**

```sql
-- Tenant / unidad de aislamiento
CREATE TABLE organizations (
  id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  created_at timestamptz DEFAULT now()
);

-- Subdivisión dentro de un org; siempre existe un grupo especial 'general'
CREATE TABLE groups (
  id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id  uuid NOT NULL REFERENCES organizations(id),
  name    text NOT NULL,
  is_general boolean NOT NULL DEFAULT false,
  created_at  timestamptz DEFAULT now(),
  UNIQUE (org_id, name)
);

-- Relación usuario ↔ org + grupo, con rol
CREATE TABLE memberships (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id  uuid NOT NULL REFERENCES auth.users(id),
  org_id   uuid NOT NULL REFERENCES organizations(id),
  group_id uuid NOT NULL REFERENCES groups(id),
  role     text NOT NULL CHECK (role IN ('org:owner', 'group:manager', 'group:member')),
  UNIQUE (user_id, org_id, group_id)
);

-- Definición de agente, scoped a org + grupo
CREATE TABLE agents (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id      uuid NOT NULL REFERENCES organizations(id),
  group_id    uuid NOT NULL REFERENCES groups(id),
  name        text NOT NULL,
  source      text NOT NULL CHECK (source IN ('blank','template','ai-assist','github')),
  config      jsonb,
  created_at  timestamptz DEFAULT now()
);

-- Historial de runs / conversaciones
CREATE TABLE runs (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id   uuid NOT NULL REFERENCES agents(id),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  group_id   uuid NOT NULL REFERENCES groups(id),
  user_id    uuid NOT NULL REFERENCES auth.users(id),
  prompt     text NOT NULL,
  result     text,
  status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','done','error')),
  created_at timestamptz DEFAULT now()
);
```

**Helper `security definer`** — deriva el contexto del tenant desde `memberships`, nunca desde JWT claims:

```sql
CREATE OR REPLACE FUNCTION current_tenant()
RETURNS TABLE(org_id uuid, group_id uuid, role text)
LANGUAGE sql SECURITY DEFINER AS $$
  SELECT m.org_id, m.group_id, m.role
  FROM memberships m
  WHERE m.user_id = auth.uid()
  LIMIT 1;
$$;
```

### 4.3 Políticas RLS

**Invariante central:**

```
Un usuario lee datos donde: org_id = su_org AND group_id ∈ { general, su_grupo }
El org:owner lee datos donde: org_id = su_org (todos los grupos)
```

**Policy base para tablas tenant-scoped:**

```sql
CREATE POLICY tenant_isolation ON agents FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    -- org:owner ve todos los grupos de su org
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR
    -- otros roles: solo su grupo y general
    group_id IN (
      SELECT g.id FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
    )
  )
);
```

**Restricción de conversaciones para `org:owner`** — el owner no puede leer runs de otros grupos:

```sql
CREATE POLICY runs_isolation ON runs FOR SELECT USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND group_id IN (
    SELECT g.id FROM groups g
    WHERE g.org_id = (SELECT org_id FROM current_tenant())
      AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
  )
);
```

### 4.4 Storage layout y RLS

```
houston/{org_id}/{group_id}/agents/{agent_id}/   ← archivos del agente (group-scoped)
houston/{org_id}/general/                         ← conocimiento org-wide
templates/                                        ← catálogo global read-only (no tenant-scoped)
```

**Storage RLS:** misma lógica de `org = mine AND group ∈ {general, mine}` aplicada a los prefijos del bucket. Los `templates/` son read-only para todos los usuarios autenticados.

### 4.5 RBAC — operaciones por rol

| Operación | `org:owner` | `group:manager` | `group:member` |
|---|---|---|---|
| CRUD de grupos | ✓ | — | — |
| CRUD de usuarios (memberships) | ✓ | solo su grupo | — |
| CRUD de agentes | cualquier grupo del org | solo su grupo | solo su grupo |
| Correr agentes | cualquier grupo del org | solo su grupo | solo su grupo |
| Leer conversaciones | solo las propias | solo las propias | solo las propias |

RBAC se implementa en middleware Go: el handler lee el rol desde `current_tenant()` y retorna 403 si la operación no está permitida.

### 4.6 Create-agent flow

```
POST /v1/agents
  → middleware: derivar (org_id, group_id, role) desde memberships
  → verificar role ≥ group:manager; si no → 403
  → switch source:
      blank      → skeleton CLAUDE.md
      template   → copiar templates/{template_id}/* a houston/{org}/{group}/agents/{id}/
      ai-assist  → generar CLAUDE.md via Claude (cheap model, one-shot)
      github     → fetch repo houston.json + CLAUDE.md + .houston/
  → INSERT INTO agents (org_id, group_id, ...)
  → upload archivos a houston/{org_id}/{group_id}/agents/{agent_id}/
  → 201 { id }
```

### 4.7 Run-agent flow (con concurrencia)

```
POST /v1/agents/{id}/runs
  → middleware: derivar (org_id, group_id, role); verificar acceso al agente
  → INSERT INTO runs (status: 'pending')
  → goroutine:
      ├── lanzar Claude Code CLI como subprocess (workspace temporal aislado)
      └── en paralelo: descargar TODOS los objetos de:
              houston/{org_id}/{group_id}/agents/{agent_id}/
              houston/{org_id}/general/
          (RLS garantiza que solo se retornan objetos del tenant correcto)
  → cuando ambas ramas completan: hidratar workspace + enviar prompt a Claude Code
  → Claude Code hace inferencia → retorna resultado
  → UPDATE runs SET result=..., status='done'
  → 200 { result }
```

**Concurrencia:** cada run crea un directorio temporal único (`/tmp/houston-run-{uuid}/`). Múltiples runs pueden correr simultáneamente — no comparten filesystem. El estado del agente en Supabase es la fuente de verdad; el disco es throwaway.

### 4.8 BYOA provider relay

**Hallazgo de investigación (repo Houston):** `login_relay.rs` **NO es un binario invocable como subprocess** — es una library Rust acoplada al event loop Axum/Tokio de `houston-engine-server`. El crate `houston-engine-core` no tiene targets `[[bin]]`. No se puede invocar desde Go via stdin/stdout pipes.

**Decisión MVP: Opción A — API key por org.**

- Cada org registra su API key de Claude en una tabla `org_credentials` en Supabase (scoped a `org_id`, cifrada, RLS-protegida).
- El orquestador lee la key del org en cada run y la inyecta en el workspace de Claude Code como variable de entorno (`ANTHROPIC_API_KEY`).
- Sin OAuth relay. El flujo de conexión de cuenta completo (BYOA con browser redirect) queda diferido a post-MVP.
- El leak test y el RBAC test no dependen de BYOA — prueban aislamiento de datos, no de billing.

```go
// Lectura de API key del org en cada run
creds, err := db.GetOrgCredentials(ctx, orgID)
if err != nil || creds.AnthropicKey == "" {
    return errors.New("org has no provider key configured")
}
cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+creds.AnthropicKey)
```

La tabla `org_credentials` se agrega al schema de Task 0001 y la task 0009 se redefine como el endpoint de gestión de keys (CRUD de la credential, con validación de formato).

### 4.9 Auth — Supabase project propio

**Hallazgo de investigación:** las credenciales de Supabase de Houston están baked-in en el build de Tauri (`SUPABASE_URL` y `SUPABASE_ANON_KEY` reemplazados por Vite en build time). No tenemos acceso al proyecto Supabase de Houston.

**Decisión:** crear un **Supabase project propio** para Houston 2.0. Setup:
1. Provisionar nuevo proyecto en Supabase (cloud o CLI local).
2. Configurar Google OAuth en GCP Console → registrar como provider en Supabase Auth.
3. Redirect URI: `http://localhost:{puerto}/auth/callback` (web loopback, no deep link Tauri).
4. El orquestador Go maneja el PKCE exchange con el JWT de Supabase.

**Reuso del patrón de Houston:** el flujo PKCE + Google SSO es el mismo; solo cambian las credenciales (URL y anon key son nuestras) y el callback URI (loopback en lugar del deep link `houston://auth-callback`).

## §5. Alternativas consideradas

| Decisión | Alternativa descartada | Razón |
|---|---|---|
| RLS derivado de `memberships` (security definer) | RLS desde JWT claims | JWT claims requieren token refresh al cambiar de grupo; memberships es la única fuente de verdad |
| Go para el orquestador | TypeScript (Vercel AI SDK) | Go tiene mejor manejo nativo de subprocesos concurrentes y produce un binario único sin dependencias |
| Full-context (sin RAG) | pgvector semántico desde el MVP | Aislamiento correcto primero; retrieval eficiente es deuda técnica aceptada y documentada |
| Supabase project propio | Reuso del project de Houston | Las credenciales de Houston están baked-in en el build de Tauri (Vite compile-time substitution); no tenemos acceso |
| BYOA MVP: API key por org (Opción A) | Relay OAuth en Go / invocar login_relay.rs | login_relay.rs es library Rust no invocable. Opción A es suficiente para el isolation cut; relay completo es post-MVP |
| HTTP-only en MVP | WebSocket desde el inicio | No hay UI que necesite streaming en MVP; WebSocket es aditivo, no bloqueante |

## §6. Impacto transversal

- **Seguridad:** el aislamiento está en la capa de datos (RLS), no solo en la aplicación. Un bug en el orquestador no puede retornar datos de otro tenant si RLS está correctamente configurado. El leak test es el gate de aceptación.
- **Performance:** sin RAG en MVP — se envía todo el contexto. Aceptado como deuda técnica. Workspace por run = overhead de disco, mitiga con cleanup automático post-run.
- **Observabilidad:** sin dashboards en MVP. Los runs tienen `status` en Supabase y son auditables via SQL.
- **Backward compatibility:** rutas `/v1/*` compatibles con Houston; agregar WebSocket es aditivo.

## §7. Granularidad y descomposición en Tasks

**Estrategia elegida:** `1 Task = 1 slice vertical entregable` — cada task produce funcionalidad testeable en aislamiento y tiene dependencias explícitas.

**Justificación:** el MVP tiene un número acotado de capas ortogonales (datos, auth, RBAC, flows, tests). Granularidad más gruesa haría los PRs inrevisables; más fina (una migration por tabla) no añade valor de aislamiento.

| id | slug | scope | depende de |
|---|---|---|---|
| 0001 | db-schema | Migraciones Supabase: `organizations`, `groups`, `memberships`, `agents`, `runs` | — |
| 0002 | rls-postgres | Políticas RLS en todas las tablas tenant-scoped + helper `current_tenant()` | 0001 |
| 0003 | storage-layout | Bucket structure + Storage RLS + seed template `sales` en `templates/` | 0001 |
| 0004 | auth-identity | Supabase project PROPIO + Google SSO PKCE + loopback redirect (patrón Houston, credenciales propias) | 0001 |
| 0005 | rbac-middleware | Go middleware: deriva `(org_id, group_id, role)` + enforcement 403 por rol | 0002, 0004 |
| 0006 | orchestrator-foundation | Go HTTP server, rutas `/v1/*`, context request, logging | 0005 |
| 0007 | create-agent-flow | `POST /v1/agents` (blank, template, ai-assist, github) | 0003, 0006 |
| 0008 | run-agent-flow | `POST /v1/agents/{id}/runs` + Claude Code subprocess + workspace aislado + concurrencia | 0006, 0007 |
| 0009 | provider-credentials | CRUD de API keys por org (`org_credentials`): registrar, rotar, validar formato. Key inyectada como `ANTHROPIC_API_KEY` en cada run. | 0002, 0006 |
| 0010 | seed-fixture | Script de seed: 2 orgs / 2 grupos / 3 usuarios / 3 roles + helpers para tests | 0005 |
| 0011 | leak-test | Acceptance gate: actuar como Usuario A, afirmar 0 rows / 0 objetos de Org B | 0008, 0010 |
| 0012 | rbac-test | Acceptance gate: operaciones fuera de rol retornan 403 | 0008, 0010 |

## §8. Declaración POA

> Estas tablas son la fuente que `plan-workflow` usa para derivar `workflow.md`.

### Agents

La factory estándar ya está instalada. Todas las tasks de Houston 2.0 se construyen con el ciclo: `curator → specter → qa → coder → reviewer → tester → pr → auditor`.

| # | Agent | Capa | Task donde se usa |
|---|---|---|---|
| 1 | `researcher` | Transversal | Research inicial de stack Go + Supabase |
| 2 | `tl` | Cross-cutting | Orquestación del ciclo completo |
| 3 | `planner` | Bootstrap | Ya usado (este RFC) |
| 4 | `curator` | Specify | Pulir What/Why/How por task |
| 5 | `specter` | Specify | Materializar specs de tasks |
| 6 | `qa` | Plan | Escribir tests antes de implementar |
| 7 | `coder` | Implement | Implementar cada task |
| 8 | `reviewer` | Implement | Code review |
| 9 | `tester` | Validate | Correr tests |
| 10 | `pr` | Validate | Abrir PR |
| 11 | `auditor` | Validate | Validar Spec↔Código |

### Skills, Hooks y Commands

Se usa el conjunto estándar de la factory (ya instalado):
- **Skills:** `research-topic`, `draft-prd`, `draft-rfc`, `plan-workflow`, `propose-agents`, `polish-idea`, `write-spec`, `spec-lint`, `author-tests`, `verify-contract`
- **Hooks:** `permissions-guard` (SessionStart), `pre-spec-validate` (PreToolUse Write), `pre-commit-contract` (git pre-commit), `post-merge-bump` (git post-merge)
- **Commands:** `/factory-init`, `/workflow-review`, `/task-run`, `/agent-new`, `/spec-new`, `/spec-bump`
- **MCPs:** ninguno en la factory base. Acceso a Supabase vía Go SDK, no MCP.

## §9. Plan de testing

- **Unit:** políticas RLS individuales (`pgTAP` o `psql`); helper `current_tenant()`; RBAC middleware (mock de memberships); lógica de derivación de contexto en Go.
- **Integration:** flujo create-agent end-to-end (blank + template); flujo run-agent contra Claude Code CLI real; BYOA relay (API key o relay Go según decisión §11).
- **Acceptance:** `scripts/leak-test.sh` — seed 2 orgs, actuar como Usuario A, afirmar 0 rows / 0 objetos de Org B (Tasks 0011, 0012). Ambos tests son el gate final del MVP.

## §10. Riesgos

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| RLS misconfiguration que bypasea aislamiento | Media | Crítico | Leak test como CI gate; review de policies en cada PR |
| Race condition en workspaces concurrentes | Baja | Alto | Directorio `/tmp/houston-run-{uuid}/` único por run; tests de concurrencia en Task 0008 |
| API key inválida o ausente en run | Baja | Medio | Validar en Task 0009 que la key tiene formato correcto; error 400 accionable en run si falta |
| Supabase project nuevo — config Google OAuth desde cero | Baja | Medio | Seguir patrón Houston (PKCE + loopback redirect); GCP Console + Supabase Auth bien documentados |
| `current_tenant()` retorna múltiples filas (usuario en varios grupos) | Media | Alto | `LIMIT 1` en el helper MVP; diseño multi-grupo es post-MVP |
| Claude Code CLI breaking change | Baja | Alto | Pin de versión en go.mod / Makefile; detectado en tests de integración de Task 0008 |

## §11. Preguntas abiertas

- [x] ~~BYOA provider — ¿Opción A o Opción B?~~ **Resuelto:** Opción A. API key por org en tabla `org_credentials` (RLS-scoped, cifrada). Task 0009 renombrada a `provider-credentials`. Relay OAuth completo diferido a post-MVP.

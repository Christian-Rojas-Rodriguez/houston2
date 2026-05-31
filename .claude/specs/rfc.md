---
version: 0.1.1
status: approved
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
│  BYOA MVP: API key por org (login_relay.rs        │
│  es library Rust, no binario invocable)           │
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

-- API keys de proveedor por org (BYOA MVP)
CREATE TABLE org_credentials (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id         uuid NOT NULL REFERENCES organizations(id) UNIQUE,
  anthropic_key  bytea NOT NULL,   -- pgp_sym_encrypt(plaintext, vault_key); bytea para compatibilidad con pgp_sym_decrypt
  created_at     timestamptz DEFAULT now(),
  updated_at     timestamptz DEFAULT now()
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
El org:owner lee datos donde: org_id = su_org (todos los grupos), excepto runs ajenos
```

**Policy base para tablas tenant-scoped (agents, groups, memberships):**

```sql
CREATE POLICY tenant_isolation ON agents FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR
    group_id IN (
      SELECT g.id FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
    )
  )
);
```

**Restricción de runs** — nadie lee runs ajenos, incluido `org:owner`:

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

**`org_credentials`** — solo el `org:owner` puede leer/escribir las keys de su org:

```sql
CREATE POLICY credentials_owner ON org_credentials FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (SELECT role FROM current_tenant()) = 'org:owner'
);
```

### 4.4 Storage layout y RLS

```
houston/{org_id}/{group_id}/agents/{agent_id}/   ← archivos del agente (group-scoped)
houston/{org_id}/general/                         ← conocimiento org-wide
templates/                                        ← catálogo global read-only (no tenant-scoped)
```

Storage RLS: misma lógica `org = mine AND group ∈ {general, mine}` en prefijos de bucket. `templates/` es read-only para todos los usuarios autenticados.

### 4.5 RBAC — operaciones por rol

| Operación | `org:owner` | `group:manager` | `group:member` |
|---|---|---|---|
| CRUD de grupos | ✓ | — | — |
| CRUD de usuarios (memberships) | ✓ | solo su grupo | — |
| CRUD de agentes | cualquier grupo del org | solo su grupo | solo su grupo |
| Correr agentes | cualquier grupo del org | solo su grupo | solo su grupo |
| Leer conversaciones (runs) | solo las propias | solo las propias | solo las propias |
| Gestionar API keys (`org_credentials`) | ✓ | — | — |

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
  → leer API key: SELECT anthropic_key FROM org_credentials WHERE org_id = mine
  → INSERT INTO runs (status: 'pending')
  → goroutine:
      ├── lanzar Claude Code CLI como subprocess en workspace /tmp/houston-run-{uuid}/
      │   con ANTHROPIC_API_KEY inyectada
      └── en paralelo: descargar TODOS los objetos de:
              houston/{org_id}/{group_id}/agents/{agent_id}/
              houston/{org_id}/general/
          (RLS garantiza que solo se retornan objetos del tenant correcto)
  → cuando ambas ramas completan: hidratar workspace + enviar prompt a Claude Code
  → Claude Code hace inferencia → retorna resultado
  → UPDATE runs SET result=..., status='done'
  → 200 { result }
```

**Concurrencia:** cada run usa `/tmp/houston-run-{uuid}/` único. No comparten filesystem. Supabase es la fuente de verdad; el disco es throwaway.

### 4.8 BYOA provider — API key por org (Opción A)

`login_relay.rs` de Houston **no es invocable como subprocess** — es una library Rust acoplada a Axum/Tokio sin targets `[[bin]]`. Decisión MVP: **API key por org** en tabla `org_credentials` (RLS-protegida, cifrada en reposo).

```go
creds, err := db.GetOrgCredentials(ctx, orgID)
if err != nil || creds.AnthropicKey == "" {
    return errors.New("org has no provider key configured")
}
cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+creds.AnthropicKey)
```

Relay OAuth completo (browser redirect + paste-back) diferido a post-MVP.

### 4.9 Auth — Supabase project propio

Las credenciales de Houston están baked-in en el build de Tauri — sin acceso al proyecto. Houston 2.0 usa un **Supabase project propio**:

1. Provisionar nuevo proyecto Supabase (cloud o CLI local).
2. Configurar Google OAuth en GCP Console → registrar como provider en Supabase Auth.
3. Redirect URI: `http://localhost:{puerto}/auth/callback` (web loopback, no deep link Tauri).
4. El orquestador Go maneja el PKCE exchange y valida el JWT de Supabase.

Reuso de Houston: el patrón PKCE + Google SSO es idéntico; solo cambian credenciales y redirect URI.

## §5. Alternativas consideradas

| Decisión | Alternativa descartada | Razón |
|---|---|---|
| RLS derivado de `memberships` (security definer) | RLS desde JWT claims | JWT claims requieren token refresh al cambiar de grupo; memberships es la única fuente de verdad |
| Go para el orquestador | TypeScript (Vercel AI SDK) | Go tiene mejor manejo nativo de subprocesos concurrentes y produce un binario único sin dependencias |
| Full-context (sin RAG) | pgvector semántico desde el MVP | Aislamiento correcto primero; retrieval eficiente es deuda técnica aceptada |
| Supabase project propio | Reuso del project de Houston | Credenciales de Houston baked-in en build Tauri (Vite compile-time substitution) — sin acceso |
| BYOA MVP: API key por org | Relay OAuth en Go / login_relay.rs | login_relay.rs es library Rust sin binario invocable. API key suficiente para el isolation cut |
| HTTP-only en MVP | WebSocket desde el inicio | No hay UI que necesite streaming; WebSocket es aditivo post-MVP |

## §6. Impacto transversal

- **Seguridad:** aislamiento en la capa de datos (RLS). Un bug en el orquestador no puede retornar datos de otro tenant si RLS está correctamente configurado. Leak test es el gate de aceptación.
- **Performance:** sin RAG — se envía todo el contexto. Deuda técnica aceptada. Workspace por run se limpia post-run.
- **Observabilidad:** runs auditables via SQL en Supabase. Sin dashboards en MVP.
- **Backward compatibility:** rutas `/v1/*` compatibles con Houston; WebSocket es aditivo.

## §7. Granularidad y descomposición en Tasks

**Estrategia:** `1 Task = 1 slice vertical entregable` con dependencias explícitas.

| id | slug | scope | depende de | estado |
|---|---|---|---|---|
| 0001 | db-schema | Migraciones: `organizations`, `groups`, `memberships`, `agents`, `runs`, `org_credentials` | — | ✅ done |
| 0002 | rls-postgres | Políticas RLS + helper `current_tenant()` + `get_org_anthropic_key()` en todas las tablas tenant-scoped | 0001 | ✅ done |
| 0003 | storage-layout | Bucket structure + Storage RLS + seed template `sales` en `templates/` | 0001 | 🔲 skeleton |
| 0004 | auth-identity | Supabase project propio + Google SSO PKCE + loopback redirect en Go | 0001 | ✅ implemented |
| 0005 | rbac-middleware | Go middleware: deriva `(org_id, group_id, role)` + enforcement 403 por rol | 0002, 0004 | ✅ implemented |
| 0006 | orchestrator-foundation | Go HTTP server, rutas `/v1/*`, request context, logging | 0005 | ✅ implemented |
| 0007 | create-agent-flow | `POST /v1/agents` (blank, template, ai-assist, github) — MVP: solo sube `CLAUDE.md` | 0003, 0006 | ✅ implemented |
| 0008 | run-agent-flow | `POST /v1/agents/{id}/runs` + Claude Code subprocess + workspace aislado + concurrencia | 0006, 0007 | 🔲 skeleton |
| 0009 | provider-credentials | CRUD `/v1/orgs/{id}/credentials` — registrar, rotar, validar API key | 0002, 0006 | 🔲 skeleton |
| 0010 | seed-fixture | Script seed: 2 orgs / 2 grupos / 3 usuarios / 3 roles + helpers para tests | 0005 | 🔲 skeleton |
| 0011 | leak-test | Acceptance gate: 0 rows / 0 objetos de otro tenant retornados a Usuario A | 0008, 0010 | 🔲 skeleton |
| 0012 | rbac-test | Acceptance gate: operaciones fuera de rol retornan 403 | 0008, 0010 | 🔲 skeleton |

## §8. Declaración POA

### Agents

Factory estándar instalada. Ciclo por task: `curator → specter → qa → coder → reviewer → tester → pr → auditor`.

| # | Agent | Capa | Rol en Houston 2.0 |
|---|---|---|---|
| 1 | `researcher` | Transversal | Research de stack Go + Supabase por task |
| 2 | `tl` | Cross-cutting | Orquesta el ciclo completo |
| 3 | `planner` | Bootstrap | Usado en este Bootstrap |
| 4 | `curator` | Specify | Pulir What/Why/How por task |
| 5 | `specter` | Specify | Materializar specs |
| 6 | `qa` | Plan | Tests antes de implementar |
| 7 | `coder` | Implement | Implementar cada task |
| 8 | `reviewer` | Implement | Code review |
| 9 | `tester` | Validate | Correr tests |
| 10 | `pr` | Validate | Abrir PR |
| 11 | `auditor` | Validate | Validar Spec↔Código |

### Skills, Hooks, Commands, MCPs

Conjunto estándar de la factory (ya instalado):
- **Skills:** `research-topic`, `polish-idea`, `write-spec`, `spec-lint`, `author-tests`, `verify-contract`
- **Hooks:** `permissions-guard`, `pre-spec-validate`, `pre-commit-contract`, `post-merge-bump`
- **Commands:** `/task-run`, `/spec-new`, `/spec-bump`, `/agent-new`
- **MCPs:** ninguno. Supabase vía Go SDK; Claude Code vía subprocess.

## §9. Plan de testing

- **Unit:** políticas RLS (`pgTAP`); helper `current_tenant()`; RBAC middleware (mock memberships); validación de API key (formato).
- **Integration:** create-agent end-to-end (blank + template); run-agent contra Claude Code CLI real; PKCE auth flow.
- **Acceptance:** Tasks 0011 (leak-test) y 0012 (rbac-test) son el gate final del MVP.

## §10. Riesgos

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| RLS misconfiguration que bypasea aislamiento | Media | Crítico | Leak test como CI gate; review de policies en cada PR |
| Race condition en workspaces concurrentes | Baja | Alto | `/tmp/houston-run-{uuid}/` único por run; cleanup automático post-run |
| API key ausente o inválida en run | Baja | Medio | Validar formato en Task 0009; error 400 accionable si falta |
| Config Google OAuth desde cero (nuevo project) | Baja | Medio | Patrón Houston bien documentado; GCP Console + Supabase Auth guías oficiales |
| `current_tenant()` con múltiples memberships | Media | Alto | `LIMIT 1` en MVP; multi-group es post-MVP |
| Claude Code CLI breaking change | Baja | Alto | Pin de versión; detectado en tests de integración Task 0008 |

## §11. Decisiones de implementación (Tasks 0004–0007)

Decisiones tomadas durante la implementación que completan o ajustan el diseño del RFC:

| Decisión | Tarea | Detalle |
|---|---|---|
| `ContextWithJWT` + `JWTFromContext` | 0004 | El JWT del usuario se almacena en el request context (además del `user_id`) para que handlers downstream puedan usarlo en calls a Supabase sin recibirlo como parámetro explícito. |
| `anthropic_key bytea` en `org_credentials` | 0001 | `pgp_sym_encrypt` retorna `bytea`; almacenar como `text` requeriría conversión base64 que rompe `pgp_sym_decrypt`. La columna es `bytea` en la migración real. |
| `get_org_anthropic_key()` security-definer | 0002 (pendiente) | Task 0007 (`ai-assist`) llama a `/rpc/get_org_anthropic_key` para obtener la clave desencriptada server-side. Task 0002 debe crear esta función. Mismo patrón que `current_tenant()`. |
| `AgentStore` interface en `internal/handlers/` | 0007 | Hexagonal: la interfaz vive en el paquete consumidor (handlers), no en un paquete de Supabase. `SupabaseAgentStore` es el adaptador concreto. Mismo patrón que `TenantQuerier` en 0005. |
| MVP scope `CLAUDE.md` solamente | 0007 | Template y GitHub sources solo fetched/suben `CLAUDE.md`. La copia de `templates/{id}/*` y fetch de `.houston/` es post-MVP. Documentado en Out of scope de Task 0007. |
| `RequireRole(RoleManager)` en `POST /v1/agents` | 0006→0007 | Task 0006 registró el stub con `RequireRole(RoleMember)`. Task 0007 eleva el gate a `RoleManager` al wirear el handler real. El test AC4 de 0006 fue actualizado para reflejar el cambio. |
| `AnthropicBaseURL` inyectable en handler | 0007 | `CreateAgent(store, anthropicURL string)` — URL vacía defaultea a `https://api.anthropic.com`. Permite mockear la API en tests de integración sin modificar código de producción. |
| Go 1.22 Enhanced ServeMux | 0006 | `go.mod` bumpeado de 1.21 → 1.22 para habilitar patrones `METHOD /path/{param}` sin router externo (gorilla/mux, chi). |

## §12. Preguntas abiertas

- [x] ~~BYOA provider — ¿Opción A o Opción B?~~ **Resuelto:** Opción A. API key por org en `org_credentials`. Relay OAuth diferido a post-MVP.
- [ ] **`get_org_anthropic_key()` SQL** — Task 0002 debe definir esta función security-definer. ¿Usa Vault de Supabase o `pgp_sym_decrypt` con una master key en env var?
- [ ] **Bucket name** — Task 0003 debe confirmar el nombre del bucket Storage (`houston` vs `houston-agents`). Task 0007 asume `houston` con path `{org_id}/{group_id}/agents/{agent_id}/CLAUDE.md`.

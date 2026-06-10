---
version: 0.1.0
status: approved
---

# PRD: Houston 2.0 — Plataforma multi-tenant de AI agents

| Campo | Valor |
|---|---|
| **Autor** | Christian Rojas |
| **Stakeholders** | Organizaciones e ingenieros que necesitan hospedar AI agents para múltiples equipos |
| **Target release** | v0.1.0 (Isolation MVP — local) |

## §1. TL;DR

Houston 2.0 es una plataforma para hospedar AI agents en modo multi-tenant. Un único orquestador sirve agentes para múltiples organizaciones garantizando que ninguna organización pueda acceder a los datos de otra, mientras que los agentes dentro de la misma organización comparten una base de conocimiento estructurada y aislada por grupo. El acceso dentro de una organización está gobernado por roles: desde el CEO que ve todo, hasta el analista que opera únicamente dentro de su grupo.

El primer entregable es una **demo de aislamiento**: la plataforma corriendo para 2 orgs / 2 grupos / 3 usuarios con roles distintos, con un leak test automatizado que falla si el aislamiento se rompe.

## §2. Problema

- **¿Qué problema resolvemos?** Los sistemas actuales de agentes AI (incluyendo Houston, la referencia) son single-tenant y local-first: no tienen concepto de `org_id`/tenant, ni de roles dentro de una organización. Compartir infraestructura entre múltiples organizaciones sin garantías de aislamiento introduce riesgo real de data leakage entre tenants. Y sin roles, no hay forma de expresar que el CEO ve todo pero el analista solo ve su grupo.
- **¿Cómo lo sabemos?** Houston (https://github.com/gethouston/houston) tiene 15 crates de engine sin ningún concepto `org_id`/tenant; sus módulos `teams/` y `cloud/` son stubs de README sin implementar. El multi-tenancy y el RBAC son gaps explícitos.
- **¿A quién le pasa?** Organizaciones y equipos de ingeniería que quieren usar agentes AI para múltiples grupos internos (ej. `management` y `analysts`) o múltiples clientes externos, con control granular de quién puede hacer qué.

## §3. Objetivos y métricas de éxito

**Goals:**
- Probar que dos organizaciones corriendo en la misma instancia son completamente opacas entre sí.
- Probar que dentro de una org, los grupos tienen conocimiento separado salvo el `general` compartido.
- Probar que los roles (owner, manager, member) restringen correctamente qué operaciones puede hacer cada usuario.
- Demostrar que el diseño escala a N orgs / N grupos / N usuarios / N roles sin cambios arquitecturales.
- Proveer dos casos de uso tenant-aislados: **crear un agente** y **correr un agente**.

**No-goals:**
- Una interfaz de usuario final (desktop, mobile o web) — la demo MVP usa curl y scripts.
- Billing real ni reselling de tokens de AI — los orgs conectan sus propias cuentas de proveedor (BYOA).
- Hosting en cloud / sandboxes efímeros — el MVP corre local.
- Búsqueda semántica / RAG — el MVP envía todo el contexto org+grupo sin selección.
- Streaming en tiempo real — HTTP request→response solamente (no WebSocket en MVP).
- Sharing controlado entre grupos u organizaciones (`context_shares`) — post-MVP.

**Métricas de éxito:**
- North star: el **leak test** pasa. Ninguna row de Postgres ni objeto de Storage de Org B es retornado a un usuario autenticado de Org A. Si el aislamiento se rompe, el test falla.
- Guardrail: RLS habilitado en **todas** las tablas tenant-scoped y en **todos** los buckets de Storage. Ninguna query bypasea RLS.
- Guardrail de roles: una operación CRUD de grupos ejecutada por un `group:member` debe ser rechazada (403), no silenciada.

## §4. Usuarios y casos de uso

### Roles dentro de una organización

Los grupos son **listas controladas por el admin de la organización** (no labels free-form). El acceso y las operaciones disponibles dependen del rol del usuario:

| Rol | Scope de visibilidad | Operaciones MVP |
|---|---|---|
| `org:owner` (CEO) | Estructura de la org — todos los grupos (sin leer conversaciones ajenas) | CRUD de grupos, CRUD de usuarios, CRUD de agentes en cualquier grupo de la org |
| `group:manager` (Management) | Su grupo + `general` de la org | CRUD de agentes en su grupo, add/remove members en su grupo |
| `group:member` (Analyst) | Su grupo + `general` de la org | Crear y correr agentes en su grupo |

**Restricciones MVP:** todos los roles tienen acceso únicamente a operaciones CRUD — no existe observabilidad en tiempo real de conversaciones ajenas ni dashboards de actividad. No existe un rol super-admin de plataforma. El `org:owner` es el único rol que cruza límites de grupo dentro de su org; no existe un rol que cruce límites de organización.

### Personas y user stories MVP

**Scope MVP concreto:** 2 organizaciones · 2 grupos · 3 usuarios:
- Usuario A → Org 1 / Grupo 1 / rol `group:member`
- Usuario B → Org 1 / Grupo 2 / rol `group:manager`
- Usuario C → Org 2 / Grupo 1 / rol `org:owner`

**User stories:**
- Como `group:member` (Usuario A, Org 1 / Grupo 1), quiero crear y correr agentes dentro de mi grupo, sin que Org 2 ni Grupo 2 sean visibles o accesibles para mí.
- Como `group:manager` (Usuario B, Org 1 / Grupo 2), quiero gestionar los miembros y agentes de mi grupo, sin poder ver ni modificar el Grupo 1 ni ningún dato de Org 2.
- Como `org:owner` (Usuario C, Org 2 / Grupo 1), quiero tener visibilidad completa de mi organización y poder crear grupos y gestionar usuarios, sin que ningún dato de Org 1 sea accesible.
- Como auditor / tester, quiero correr el **leak test** y ver que cero filas y cero objetos de otro tenant son retornados, y que las operaciones fuera del rol son rechazadas con 403.

## §5. Requisitos

### P0 — Must have (MVP)

- Modelo de datos multi-tenant en Supabase Postgres: `organizations`, `groups`, `memberships` (con campo `role`), `agents`, `prompts/conversations`. Toda fila tenant-scoped lleva `org_id` + `group_id`.
- **Roles en memberships**: `org:owner`, `group:manager`, `group:member`. Las operaciones CRUD de grupos y gestión de usuarios requieren `org:owner`. La gestión del propio grupo requiere `group:manager` o superior. MVP: solo operaciones CRUD — sin observabilidad avanzada ni dashboards. No existe super-admin de plataforma.
- **Grupos controlados por admin**: solo usuarios con rol `org:owner` pueden crear, renombrar o eliminar grupos. Los grupos no son free-form; son entidades administradas.
- **RLS en Postgres**: policy `org = mine AND group ∈ { general, mine }` en todas las tablas tenant-scoped, derivada de `memberships`, no de JWT claims. `org:owner` puede leer todos los grupos de su org.
- **RLS en Storage**: misma policy en bucket paths `houston/{org_id}/{group_id}/...` y `houston/{org_id}/general/...`.
- **Capa de knowledge compartida**: bucket path `templates/` como catálogo global read-only. Instanciar un template copia sus archivos al prefix tenant del org/grupo.
- **Create-agent flow**: POST `/v1/agents` con `source ∈ { blank, template, ai-assist, github }`. Agente scoped a org+grupo desde su creación. Solo `group:manager` o superior puede crear agentes en el grupo.
- **Run-agent flow**: POST `/v1/agents/{id}/runs`. Orquestador reúne TODO el org+grupo context (sin RAG), hidrata Claude Code, envía prompt, retorna respuesta. Soporta **concurrencia de sesiones**.
- **Concurrencia**: workspace de Claude Code aislado por run (directorio temporal). El orquestador gestiona múltiples subprocesos simultáneos sin race conditions.
- **El leak test pasa**: seed 2 orgs / 2 grupos / 3 usuarios con roles distintos; afirmar que cero filas y cero objetos de otro tenant son retornados; afirmar que operaciones fuera del rol retornan 403.
- **Demo vía curl y scripts**: no se requiere UI.
- Reuso de Houston: login (Supabase Auth + Google SSO PKCE), formato de agente (`houston.json` + `CLAUDE.md`), helpers in-instance, login relay BYOA como subprocess.

### P1 — Should have

- `security definer` helper en Postgres para derivar `(org_id, group_id, role)` desde `memberships` sin trustar JWT claims.
- Catálogo `templates` con al menos un template semilla (ej. `sales`).
- Rutas `/v1/*` compatibles con Houston para que WebSocket sea aditivo post-MVP.
- Seed fixture reproducible (3 usuarios / 2 orgs / 2 grupos / 3 roles).
- **Gestión de usuarios manual**: el `org:owner` crea/asigna/elimina usuarios vía API/scripts. Sin email invite ni auto-provisioning.
- Manejo de errores claro para BYOA cuando la cuenta de proveedor se revoca.

### P2 — Nice to have (post-MVP)

- `context_shares` table para sharing controlado entre grupos/orgs.
- Event sourcing para historial de contexto inmutable.
- pgvector para búsqueda semántica scoped a `{general, group}`.
- UI mínima (web form) sobre las APIs.

## §6. Experiencia de usuario (flujos)

**Flujo 1 — Crear un agente (requiere `group:manager` o `org:owner`):**
1. `POST /v1/agents` con nombre, fuente y `group_id` objetivo.
2. Orquestador Go deriva `(org_id, group_id, role)` desde `memberships`; verifica rol.
3. Según `source`: copia template / genera `CLAUDE.md` / clona repo / crea skeleton.
4. Escribe fila en `agents` + objetos bajo `houston/{org_id}/{group_id}/agents/{agent_id}/`.
5. `201 { id }`. Agente tenant-aislado desde el nacimiento.

**Flujo 2 — Correr un agente (requiere `group:member` o superior):**
1. `POST /v1/agents/{id}/runs` con prompt + archivos opcionales.
2. Orquestador persiste prompt (scoped), lanza Claude Code en workspace aislado.
3. En paralelo: descarga TODOS los objetos del prefix org+grupo (RLS garantiza scope).
4. Hidrata workspace, envía prompt, retorna `200 { result }`.

**Demo MVP:** curl + `scripts/leak-test.sh` (EXIT 0 = aislamiento íntegro).

## §7. Dependencias y riesgos

| Dependencia | Sistema | Riesgo | Mitigación |
|---|---|---|---|
| Supabase RLS + RBAC | Supabase | Bug en RLS / policies que bypasea aislamiento | Leak test + RBAC test como acceptance gates |
| Claude Code CLI | Anthropic | Breaking changes en args o output | Pin de versión; tests de integración |
| Houston `login_relay.rs` (Rust) | gethouston/houston | Relay en Rust, orquestador en Go | Invocar como subprocess (stdin/stdout piped). No reescribir. |
| Google SSO / Supabase Auth | Google + Supabase | Cambios en PKCE flow | Mismo Supabase project que Houston |
| Concurrencia de sesiones | Orquestador Go | Race conditions en estado de agente | Workspace aislado por run; Supabase es la fuente de verdad |

## §8. Rollout

- **Milestone 1 — Isolation cut (MVP, local):** modelo con org+group+role; RLS Postgres+Storage; RBAC 3 roles; create-agent + run-agent; leak test + RBAC test pasan. Local: Go + Supabase + Claude Code. Demo curl/scripts.
- **Post-MVP:** cloud hosting; RAG pgvector; observabilidad; sharing controlado; UI mínima.

## §9. Preguntas abiertas

- [x] ~~¿El `org:owner` puede ver conversaciones de todos los grupos en tiempo real?~~ Resuelto: solo CRUD en MVP.
- [x] ~~¿Existe un rol super-admin de plataforma?~~ Resuelto: no existe.
- [x] ~~¿Cómo se invita a nuevos usuarios?~~ Resuelto: gestión manual vía API/scripts.
- [x] ~~¿El `org:owner` puede leer conversaciones de otros grupos?~~ Resuelto: no puede leer conversaciones. Sí puede hacer CRUD de grupos, usuarios y agentes en cualquier grupo de la org.

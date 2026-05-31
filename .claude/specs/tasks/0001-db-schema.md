---
task: "0001"
slug: db-schema
granularity: slice
version: 0.1.0
status: draft
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

Producir las migraciones SQL de Supabase que crean las seis tablas del modelo de datos de Houston 2.0 tal como están especificadas en el RFC §4.2. Al aplicar estas migraciones, el esquema Postgres queda listo para recibir políticas RLS (Task 0002), auth (Task 0004) y todos los flows posteriores.

Artefactos observables al completar esta task:

- El directorio `supabase/migrations/` existe con un archivo de migración timestamped (convención Supabase CLI: `YYYYMMDDHHmmss_db-schema.sql`).
- Las tablas `organizations`, `groups`, `memberships`, `agents`, `runs`, y `org_credentials` existen en el esquema `public` con todas sus columnas, tipos, constraints y foreign keys.
- `memberships.user_id` hace referencia a `auth.users(id)` (esquema `auth` de Supabase).
- `org_credentials.anthropic_key` está almacenada como columna `bytea` cifrada con `pgcrypto` (`pgp_sym_encrypt`) usando una clave de cifrado gestionada como secret en Supabase Vault.
- La función helper `current_tenant()` existe en el esquema `public` como `SECURITY DEFINER`, devuelve `(org_id uuid, group_id uuid, role text)` y deriva el contexto desde `memberships` — nunca desde JWT claims.
- RLS está habilitado en cada tabla tenant-scoped (`ALTER TABLE ... ENABLE ROW LEVEL SECURITY`) sin políticas definidas aún (eso es Task 0002).
- `pgcrypto` está habilitado como extensión de Postgres en el proyecto Supabase.

## Why

- **Fundamento del multi-tenancy** (PRD §5 P0): toda la invariante de aislamiento del sistema descansa en el modelo de datos. Sin `org_id` y `group_id` en cada tabla tenant-scoped no existe base para las políticas RLS (Task 0002) ni para el RBAC middleware (Task 0005).
- **Dependencia de bloqueo para otras 11 tasks** (RFC §7): 0001 es el único nodo sin dependencias en el grafo. Todas las demás tasks del Workflow dependen directa o transitivamente de este esquema. Bloquearlo bloquea todo.
- **El cifrado pertenece a la capa de datos** (RFC §4.8): `login_relay.rs` no es invocable y la key de Anthropic no debe viajar en claro por la red ni persistirse sin cifrar. Usar `pgcrypto` en la migración garantiza que la key jamás existe en texto plano en disco, aunque la capa de aplicación tenga un bug de logging.
- **RLS habilitado desde el día 0** (PRD §5 P0): `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` debe estar en esta migración. Si se omite y se agregan datos antes de Task 0002, esos datos quedan desprotegidos. Habilitarlo ahora con `FORCE ROW LEVEL SECURITY` para superusuarios evita fugas durante el desarrollo.
- **`current_tenant()` en esta task** (RFC §4.2): la función referencia `memberships`; por tanto debe crearse en la misma migración o en una que siga inmediatamente. Task 0002 la asume existente — crearla aquí elimina ambigüedad de orden de migración.

## How

### Archivos a crear

```
supabase/
  migrations/
    20260530000000_db-schema.sql   ← migración única con todo el DDL de esta task
```

El timestamp `20260530000000` es el mínimo que ubica esta migración primera en la secuencia. El nombre puede ajustarse al instante real de creación siempre que sea el menor timestamp en `supabase/migrations/`.

### DDL completo

La migración debe contener, en este orden:

**1. Extensión pgcrypto** (RFC §4.2, decisión de cifrado `anthropic_key`):
```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

**2. Tabla `organizations`** (RFC §4.2):
```sql
CREATE TABLE organizations (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       text NOT NULL,
  created_at timestamptz DEFAULT now()
);
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
```

**3. Tabla `groups`** (RFC §4.2):
```sql
CREATE TABLE groups (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  name       text NOT NULL,
  is_general boolean NOT NULL DEFAULT false,
  created_at timestamptz DEFAULT now(),
  UNIQUE (org_id, name)
);
ALTER TABLE groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE groups FORCE ROW LEVEL SECURITY;
```

**4. Tabla `memberships`** (RFC §4.2 — FK a `auth.users`):
```sql
CREATE TABLE memberships (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id  uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  org_id   uuid NOT NULL REFERENCES organizations(id),
  group_id uuid NOT NULL REFERENCES groups(id),
  role     text NOT NULL CHECK (role IN ('org:owner', 'group:manager', 'group:member')),
  UNIQUE (user_id, org_id, group_id)
);
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
```

**5. Tabla `agents`** (RFC §4.2):
```sql
CREATE TABLE agents (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  group_id   uuid NOT NULL REFERENCES groups(id),
  name       text NOT NULL,
  source     text NOT NULL CHECK (source IN ('blank', 'template', 'ai-assist', 'github')),
  config     jsonb,
  created_at timestamptz DEFAULT now()
);
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents FORCE ROW LEVEL SECURITY;
```

**6. Tabla `runs`** (RFC §4.2):
```sql
CREATE TABLE runs (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id   uuid NOT NULL REFERENCES agents(id),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  group_id   uuid NOT NULL REFERENCES groups(id),
  user_id    uuid NOT NULL REFERENCES auth.users(id),
  prompt     text NOT NULL,
  result     text,
  status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'error')),
  created_at timestamptz DEFAULT now()
);
ALTER TABLE runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE runs FORCE ROW LEVEL SECURITY;
```

**7. Tabla `org_credentials`** (RFC §4.2 + RFC §4.8 — cifrado en reposo):
```sql
CREATE TABLE org_credentials (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id         uuid NOT NULL REFERENCES organizations(id) UNIQUE,
  anthropic_key  bytea NOT NULL,   -- pgp_sym_encrypt(plaintext, vault_key)
  created_at     timestamptz DEFAULT now(),
  updated_at     timestamptz DEFAULT now()
);
ALTER TABLE org_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_credentials FORCE ROW LEVEL SECURITY;
```

El tipo `bytea` (no `text`) es deliberado: `pgp_sym_encrypt` devuelve `bytea`; almacenarlo como `text` con `encode(…, 'base64')` agrega codificación innecesaria y rompe la compatibilidad directa con `pgp_sym_decrypt`. (Alternativa descartada: columna `text` con `encode` — innecesaria complejidad en cada read/write. RFC §5 confirma simplicidad sobre ergonomía hasta que haya un motivo real.)

**8. Función `current_tenant()`** (RFC §4.2 — base del helper para RLS en Task 0002):
```sql
CREATE OR REPLACE FUNCTION current_tenant()
RETURNS TABLE(org_id uuid, group_id uuid, role text)
LANGUAGE sql
SECURITY DEFINER
STABLE
AS $$
  SELECT m.org_id, m.group_id, m.role
  FROM memberships m
  WHERE m.user_id = auth.uid()
  LIMIT 1;
$$;
```

`STABLE` se agrega porque la función no modifica la base de datos y retorna el mismo resultado dentro de una transacción — permite al planner de Postgres cachear el resultado dentro de una query, importante para las políticas RLS de Task 0002 que la llaman múltiples veces.

### Ejecución / deployment

El proyecto Supabase ya está vinculado a un proyecto cloud real. El archivo `.temp/linked-project.json` presente en `supabase/` confirma el vínculo — no es necesario correr `supabase link`.

**Detalle del proyecto vinculado:**
- Ref: `sjstvuqdiowyzuockwds`
- Nombre: `houston2`
- Región: `us-east-2`
- CLI instalada localmente: `2.102.0`

**Flujo de aplicación:**

1. **Iteración local (antes de push):**
   ```
   supabase start           # levanta la pila local (Postgres + Auth + Storage)
   supabase db reset        # aplica todas las migraciones en `supabase/migrations/` desde cero
   ```
   Usar `supabase db reset` durante el desarrollo permite iterar sobre el DDL sin afectar el proyecto cloud.

2. **Deploy a cloud:**
   ```
   supabase db push
   ```
   `supabase db push` lee el proyecto vinculado desde `.temp/linked-project.json` y aplica las migraciones pendientes contra el proyecto `sjstvuqdiowyzuockwds`. No es necesario pasar `--project-ref` ni autenticarse interactivamente si ya existe la sesión local del CLI.

**Idempotencia:** Supabase CLI registra cada migración aplicada en la tabla interna `supabase_migrations.schema_migrations`. Re-ejecutar `supabase db push` contra el mismo proyecto omite archivos ya aplicados — la operación es idempotente al nivel de archivo de migración.

**Dependencia forward — `pgp_sym_encrypt` key (Task 0009):** La columna `org_credentials.anthropic_key` se define como `bytea` cifrada con `pgp_sym_encrypt(plaintext, vault_key)`. La `vault_key` debe existir como Vault secret en el proyecto cloud **antes** de que se escriban filas en `org_credentials`. Crear ese secret es responsabilidad de Task 0009 (`provider-credentials`). Esta task solo define el esquema; no escribe rows ni configura el Vault secret.

### Decisiones de diseño tomadas

| Decisión | Alternativa descartada | Razón |
|---|---|---|
| `anthropic_key` como `bytea` con `pgcrypto` | `text` con encrypt en capa Go | Cifrado en DB layer: si Go loggea en claro, la key nunca llega a disco sin cifrar. RFC §4.8. |
| `FORCE ROW LEVEL SECURITY` en cada tabla | Solo `ENABLE ROW LEVEL SECURITY` | Previene que conexiones de rol `superuser`/`service_role` en desarrollo bypaseen RLS accidentalmente. |
| `SECURITY DEFINER` + `STABLE` en `current_tenant()` | `VOLATILE` o sin `SECURITY DEFINER` | `STABLE` permite caché intra-query; `SECURITY DEFINER` permite que la función corra con permisos del owner y acceda a `memberships` aunque el caller no tenga SELECT directo. RFC §4.2. |
| FK `ON DELETE CASCADE` en `memberships.user_id` | Sin cascade | Si un usuario se elimina de `auth.users`, sus memberships se limpian automáticamente. Sin cascade, la FK bloquea el delete o deja rows huérfanas. |
| Un solo archivo de migración | Múltiples archivos por tabla | Las 6 tablas tienen dependencias entre sí (groups → organizations, memberships → groups, etc.); un solo archivo garantiza orden de ejecución sin depender del orden lexicográfico de nombres. |

### Dependencias de otras tasks

- Task 0002 (`rls-postgres`) asume que RLS está habilitado y `current_tenant()` existe — ambos producidos aquí.
- Task 0003 (`storage-layout`), Task 0004 (`auth-identity`), Task 0005 (`rbac-middleware`) asumen que las 6 tablas existen con sus columnas y constraints.
- Task 0009 (`provider-credentials`) escribe/lee `org_credentials.anthropic_key` y asume `bytea` con `pgp_sym_decrypt`.

## Acceptance criteria

1. El directorio `supabase/migrations/` existe y contiene exactamente un archivo `.sql` con timestamp en el nombre.
2. Aplicar la migración con `supabase db push` (o `psql` directo) finaliza sin errores en un proyecto Supabase vacío.
3. La tabla `organizations` existe con columnas: `id uuid PK`, `name text NOT NULL`, `created_at timestamptz DEFAULT now()`.
4. La tabla `groups` existe con columnas: `id uuid PK`, `org_id uuid NOT NULL FK → organizations(id)`, `name text NOT NULL`, `is_general boolean NOT NULL DEFAULT false`, `created_at timestamptz DEFAULT now()`, y un UNIQUE constraint sobre `(org_id, name)`.
5. La tabla `memberships` existe con columnas: `id uuid PK`, `user_id uuid NOT NULL FK → auth.users(id) ON DELETE CASCADE`, `org_id uuid NOT NULL FK → organizations(id)`, `group_id uuid NOT NULL FK → groups(id)`, `role text NOT NULL CHECK (IN ('org:owner','group:manager','group:member'))`, y un UNIQUE constraint sobre `(user_id, org_id, group_id)`.
6. La tabla `agents` existe con columnas: `id uuid PK`, `org_id uuid NOT NULL FK → organizations(id)`, `group_id uuid NOT NULL FK → groups(id)`, `name text NOT NULL`, `source text NOT NULL CHECK (IN ('blank','template','ai-assist','github'))`, `config jsonb`, `created_at timestamptz DEFAULT now()`.
7. La tabla `runs` existe con columnas: `id uuid PK`, `agent_id uuid NOT NULL FK → agents(id)`, `org_id uuid NOT NULL FK → organizations(id)`, `group_id uuid NOT NULL FK → groups(id)`, `user_id uuid NOT NULL FK → auth.users(id)`, `prompt text NOT NULL`, `result text`, `status text NOT NULL DEFAULT 'pending' CHECK (IN ('pending','running','done','error'))`, `created_at timestamptz DEFAULT now()`.
8. La tabla `org_credentials` existe con columnas: `id uuid PK`, `org_id uuid NOT NULL FK → organizations(id) UNIQUE`, `anthropic_key bytea NOT NULL`, `created_at timestamptz DEFAULT now()`, `updated_at timestamptz DEFAULT now()`.
9. `org_credentials.anthropic_key` es de tipo `bytea` (no `text`).
10. La extensión `pgcrypto` está instalada en el proyecto (`SELECT * FROM pg_extension WHERE extname = 'pgcrypto'` devuelve una fila).
11. RLS está habilitado en las seis tablas (`SELECT relrowsecurity FROM pg_class WHERE relname IN (...)` devuelve `true` para cada una).
12. La función `current_tenant()` existe en el esquema `public`, tiene `SECURITY DEFINER`, y al llamarla como un usuario con fila en `memberships` retorna `(org_id, group_id, role)` correctos para ese usuario.
13. La función `current_tenant()` retorna cero filas cuando el `auth.uid()` no tiene ninguna fila en `memberships`.
14. `supabase db push` ejecutado contra el proyecto vinculado (`sjstvuqdiowyzuockwds`) finaliza sin errores y sin prompts interactivos.
15. `supabase migration list` muestra la migración `<timestamp>_db-schema` como **applied** tanto en el entorno local (después de `supabase db reset`) como en el remoto (después de `supabase db push`).

## Out of scope

- **Políticas RLS**: `CREATE POLICY` en cualquier tabla es Task 0002. Esta task solo hace `ENABLE ROW LEVEL SECURITY` y `FORCE ROW LEVEL SECURITY`.
- **Bucket de Storage**: la estructura de prefijos (`houston/{org_id}/...`) y las Storage RLS policies son Task 0003.
- **Google SSO / Auth provider**: configuración del proyecto Supabase y PKCE flow son Task 0004.
- **Indexes de performance**: no se crean índices adicionales más allá de los PK implícitos. Optimización de queries es post-MVP.
- **`updated_at` trigger**: `org_credentials.updated_at` se declara pero el trigger que lo mantiene actualizado automáticamente es post-MVP (las escrituras en MVP son manuales via Task 0009).
- **Datos de seed**: ningunos. El seed fixture reproducible es Task 0010.
- **Encriptación de columnas adicionales**: solo `anthropic_key` en `org_credentials`. Ninguna otra columna requiere cifrado en el MVP.

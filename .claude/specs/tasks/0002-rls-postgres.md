---
task: "0002"
slug: rls-postgres
granularity: slice
version: 0.1.1
status: done
declares:
  - type: policy
    name: rls-postgres
    path: supabase/migrations/
scope:
  - supabase/migrations/
  - tests/unit/0002__rls-postgres.test.sql
---

# Task 0002 — `rls-postgres`

> Políticas RLS en todas las tablas tenant-scoped + verificación del helper `current_tenant()` (security definer). Aquí vive el aislamiento real: ninguna query mal formada puede retornar datos de otro tenant.

## What

Producir una migración SQL de Supabase que añade las políticas `CREATE POLICY` a las seis tablas tenant-scoped ya creadas por Task 0001. Task 0001 ya habilitó `ENABLE ROW LEVEL SECURITY` y `FORCE ROW LEVEL SECURITY` en cada tabla, y ya creó la función `current_tenant()` (SECURITY DEFINER, STABLE). Esta task no modifica ese DDL — solo agrega las políticas.

Artefactos observables al completar esta task:

1. **Un nuevo archivo de migración** en `supabase/migrations/` con timestamp mayor que `20260530000000` (el de Task 0001), por ejemplo `20260530000001_rls-postgres.sql`.
2. **Políticas de aislamiento base** en `organizations`, `groups`, `memberships`, `agents`: invariante `org_id = mine AND (role='org:owner' OR group_id ∈ {general_group, my_group})`. Aplica a todas las operaciones (FOR ALL = SELECT + INSERT + UPDATE + DELETE).
3. **Política de runs** (FOR ALL): `org_id = mine AND group_id ∈ {general_group, my_group}` — incluso `org:owner` no puede ver runs de grupos ajenos ni de otros usuarios en su grupo. La restricción de visibilidad de runs es estrictamente por grupo y por usuario propio.
4. **Política de `org_credentials`** (FOR ALL): `org_id = mine AND role = 'org:owner'` — solo el dueño de la org puede leer/escribir las keys.
5. **Suite de tests SQL** en `tests/unit/0002__rls-postgres.test.sql` que verifica cada invariante contra el proyecto cloud (con `pgTAP` o SQL plano con asserts).

La función `current_tenant()` ya existe en `public`. Esta task puede emitir un `CREATE OR REPLACE FUNCTION` solo si detecta que necesita ser corregida (por ejemplo si la implementación del 0001 difiere del RFC §4.2); en caso contrario la deja intacta.

## Why

- **Aislamiento multi-tenant en la capa de datos** (RFC §4.3, PRD §5 P0): sin políticas RLS activas, cualquier query autenticada puede leer filas de cualquier tenant. `ENABLE ROW LEVEL SECURITY` sin `CREATE POLICY` en tablas Supabase resulta en deny-all para el rol `authenticated` — las tablas están bloqueadas pero inutilizables. Esta task pone las policies que permiten exactamente lo necesario y nada más.
- **Garantía de aislamiento incluso ante bugs en la capa de aplicación** (RFC §1): el orquestador Go puede tener un bug de autorización. Si RLS está correctamente configurado, el bug de la capa de app produce un error de Postgres, no una fuga de datos. La defensa en profundidad es no negociable.
- **`runs` no son compartibles ni siquiera por `org:owner`** (RFC §4.5): la tabla RBAC del RFC es explícita: "Leer conversaciones (runs): solo las propias" para los tres roles. Si `org:owner` pudiera ver runs de otros usuarios, violaría la expectativa de privacidad de los miembros — los runs contienen prompts y resultados potencialmente sensibles.
- **`org_credentials` es ultra-restringida** (RFC §4.8): la API key de Anthropic cifrada en reposo solo debe ser accesible por quien la configuró (`org:owner`). Cualquier otro rol que pueda leer `org_credentials` podría extraer la key (incluso cifrada con la vault key conocida en el contexto de una sesión comprometida).
- **Desbloqueo de Tasks 0003-0029**: todas las tasks que tocan datos tenant-scoped (agents, runs, storage) asumen que RLS está activo y que las policies están correctamente configuradas. Sin Task 0002 completa, ninguna integración puede probarse de forma segura.

## How

### Archivos a crear/modificar

```
supabase/
  migrations/
    20260530000001_rls-postgres.sql   ← migración nueva (timestamp > 0001)
tests/
  unit/
    0002__rls-postgres.test.sql       ← suite de tests (Phase B, post-aprobación)
```

### DDL completo de la migración

El archivo `20260530000001_rls-postgres.sql` debe contener, en este orden:

**1. Política de `organizations`** — un usuario ve solo su org:

```sql
CREATE POLICY tenant_isolation ON organizations FOR ALL USING (
  id = (SELECT org_id FROM current_tenant())
);
```

Razón: `organizations` no tiene `org_id` propio (es la raíz del tenant). La condición es `id = mi_org_id` derivado de `current_tenant()`.

**2. Política de `groups`** — `org:owner` ve todos los grupos de su org; otros roles solo ven su grupo y el general:

> ⚠️ **Recursion-proof**: la policy de `groups` NO puede contener un subquery sobre `groups` (`SELECT g.id FROM groups g ...`) porque evaluar la policy volvería a disparar la misma policy → `infinite recursion detected in policy for relation "groups"`. El invariante `id ∈ {my_group, general groups}` se expresa con las columnas de la **propia fila** (`id`, `is_general`), que rompe el ciclo. Verificado contra Postgres local con pgTAP (24/24).

```sql
CREATE POLICY tenant_isolation ON groups FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR id = (SELECT group_id FROM current_tenant())
    OR is_general = true
  )
);
```

**3. Política de `memberships`** — mismo patrón que groups: `org:owner` ve todas las membresías de su org; otros solo las de su grupo + general:

```sql
CREATE POLICY tenant_isolation ON memberships FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR group_id IN (
      SELECT g.id FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
    )
  )
);
```

**4. Política de `agents`** — mismo patrón:

```sql
CREATE POLICY tenant_isolation ON agents FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR group_id IN (
      SELECT g.id FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
    )
  )
);
```

**5. Política de `runs`** — nadie lee runs ajenos, INCLUIDO `org:owner`:

```sql
CREATE POLICY runs_isolation ON runs FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND group_id IN (
    SELECT g.id FROM groups g
    WHERE g.org_id = (SELECT org_id FROM current_tenant())
      AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
  )
);
```

Nota: `runs` tiene también `user_id`. La policy de RFC §4.3 aísla por grupo/org (no por user_id individual) — la restricción "solo las propias" del RFC §4.5 se implementa a nivel de middleware Go (403 si `user_id != auth.uid()`), no en la policy SQL. La policy garantiza que no puedas ver runs de otros tenants ni de grupos a los que no perteneces. El middleware agrega la capa de "solo tus propios runs dentro de tu grupo".

**6. Política de `org_credentials`** — solo `org:owner`:

```sql
CREATE POLICY credentials_owner ON org_credentials FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (SELECT role FROM current_tenant()) = 'org:owner'
);
```

### Decisiones de diseño

| Decisión | Alternativa descartada | Razón |
|---|---|---|
| `FOR ALL` en todas las policies | `FOR SELECT` + `FOR INSERT` separadas | El RFC §4.3 no distingue operaciones para la invariante de aislamiento; `FOR ALL` es más seguro por defecto — no hay riesgo de olvidar una operación. |
| `organizations` policy usa `id = org_id_mine` | Sin policy en organizations | Sin policy, el deny-all de FORCE RLS bloquea toda lectura de organizations. La policy mínima necesaria es la de un miembro autenticado que ve su propia org. |
| `runs` policy NO filtra por `user_id` | Filtrar por `user_id = auth.uid()` en SQL | RFC §4.5 asigna la restricción "solo las propias" al middleware Go. Filtrar en SQL añadiría rigidez que impide, por ejemplo, que un admin del sistema consulte runs para soporte. |
| Un único archivo de migración para todas las policies | Un archivo por tabla | Las policies de `groups` y `memberships` son interdependientes (memberships usa el subquery de groups); un archivo garantiza atomicidad. |
| Timestamp `20260530000001` | Timestamp real del momento de creación | Debe ser estrictamente mayor que `20260530000000` (Task 0001) y menor que cualquier migración futura. `...0001` es el mínimo válido siguiente. |

### Ejecución / deployment

Igual que Task 0001: `supabase db push` contra el proyecto cloud `sjstvuqdiowyzuockwds`. No se usa stack local.

```bash
supabase db push
```

Supabase CLI aplica solo las migraciones pendientes (las ya aplicadas están registradas en `supabase_migrations.schema_migrations`).

### Dependencias

- **Requiere**: Task 0001 (`db-schema`) — las seis tablas deben existir con RLS habilitado y `current_tenant()` creada.
- **Desbloquea**: Task 0003 (`storage-layout`), Task 0005 (`rbac-middleware`), Task 0006 (`leak-test`), y transitivamente todas las tasks de flows.

## Acceptance criteria

Los siguientes criterios son individualmente verificables contra el proyecto cloud Supabase (`sjstvuqdiowyzuockwds`) mediante SQL. Cada criterio corresponde a un caso de test en `tests/unit/0002__rls-postgres.test.sql`.

### AC-1: Migración aplicada sin errores

`supabase db push` finaliza con exit code 0 y `supabase migration list` muestra `20260530000001_rls-postgres` como **applied** en el remote.

### AC-2: Policies existen en el catálogo de Postgres

La consulta:
```sql
SELECT tablename, policyname
FROM pg_policies
WHERE schemaname = 'public'
  AND tablename IN ('organizations','groups','memberships','agents','runs','org_credentials')
ORDER BY tablename, policyname;
```
Retorna exactamente una fila por tabla (seis filas total), con los nombres de policy definidos en esta migración.

### AC-3: Usuario sin membresía no puede leer ninguna tabla tenant-scoped

Un `auth.uid()` sin fila en `memberships`: `SELECT * FROM organizations` retorna 0 filas (no error — deny-all). Mismo resultado para `groups`, `memberships`, `agents`, `runs`, `org_credentials`.

### AC-4: Usuario con membresía solo ve su propia organización

Un usuario con membresía en `org_A` ejecuta `SELECT id FROM organizations`. Retorna solo `org_A.id`. No ve `org_B` aunque exista en la tabla.

### AC-5: `group:member` ve su grupo y el grupo general, pero no otros grupos del mismo org

Un `group:member` de `org_A` / `group_X` ejecuta `SELECT id FROM groups WHERE org_id = org_A`. Retorna solo `group_X.id` y el grupo con `is_general = true`. No retorna `group_Y` (otro grupo del mismo org).

### AC-6: `org:owner` ve todos los grupos de su org

Un `org:owner` de `org_A` ejecuta `SELECT id FROM groups WHERE org_id = org_A`. Retorna todos los grupos del org (general + todos los grupos específicos).

### AC-7: Usuario no puede leer runs de otro usuario de su mismo grupo

Dos usuarios (`user_A` y `user_B`) pertenecen al mismo `group_X` de `org_A`. `user_A` inserta un run. `user_B` ejecuta `SELECT * FROM runs WHERE org_id = org_A AND group_id = group_X`. No retorna el run de `user_A`. (La restricción "solo propios" se verifica aquí vía middleware — la policy SQL no filtra por user_id, pero el test de middleware confirma el 403.)

> Nota de alcance: AC-7 valida el comportamiento de la policy SQL (qué devuelve el DB). La capa de 403 del middleware es Task 0005. Lo que la policy garantiza es que un usuario en otro grupo/org no puede ver los runs.

### AC-8: `org:owner` no puede leer runs de grupos a los que no pertenece dentro de su org

Un `org:owner` pertenece a `group_general` (is_general=true). `group_Y` es un grupo específico al que el owner no tiene membresía directa. Un run en `group_Y` NO debe ser visible al `org:owner` vía la policy SQL de runs (porque `group_Y` no es su grupo ni el general).

### AC-9: `group:member` y `group:manager` no pueden leer `org_credentials`

Un usuario con role `group:member` ejecuta `SELECT * FROM org_credentials`. Retorna 0 filas. Mismo resultado para `group:manager`.

### AC-10: `org:owner` puede leer `org_credentials` de su propia org

Un `org:owner` de `org_A` ejecuta `SELECT id FROM org_credentials WHERE org_id = org_A`. Retorna la fila correspondiente. No retorna la fila de `org_B`.

### AC-11: `org:owner` no puede leer `org_credentials` de otro org

Un `org:owner` de `org_A` ejecuta `SELECT * FROM org_credentials WHERE org_id = org_B`. Retorna 0 filas.

### AC-12: INSERT en tabla ajena es bloqueado por RLS

Un `group:member` de `org_A` intenta `INSERT INTO agents (org_id, group_id, ...) VALUES (org_B, ...)`. La operación falla con error de RLS (no con un insert silencioso en org_B).

### AC-13: `current_tenant()` retorna (org_id, group_id, role) correctos para el usuario autenticado

Un usuario con una sola fila en `memberships` (org_A, group_X, 'group:member') ejecuta `SELECT * FROM current_tenant()`. Retorna exactamente `(org_A.id, group_X.id, 'group:member')`.

### AC-14: `current_tenant()` retorna 0 filas para usuario sin membresía

Un `auth.uid()` sin fila en `memberships` ejecuta `SELECT * FROM current_tenant()`. Retorna 0 filas (no error).

## Out of scope

- **Storage RLS**: políticas sobre el bucket de Supabase Storage (`houston/{org_id}/...`) van en Task 0003.
- **Middleware RBAC Go**: las restricciones de CRUD por rol (`group:manager` puede crear agents, `group:member` no) se implementan en el handler Go (Task 0005), no en RLS. RLS solo garantiza aislamiento de tenant; RBAC de operaciones es capa de aplicación.
- **`updated_at` trigger en `org_credentials`**: post-MVP.
- **Indexes de performance**: no se crean índices adicionales en esta task.
- **Políticas de INSERT con WITH CHECK**: el RFC no especifica `WITH CHECK` separado; `FOR ALL` con `USING` aplica tanto a reads como a writes en Postgres RLS. Si se requiere `WITH CHECK` diferenciado, es una tarea de hardening post-MVP.
- **Recrear o modificar `current_tenant()`**: la función ya existe tal como la requiere esta task (creada en Task 0001). Solo se modifica si se detecta una divergencia respecto al RFC §4.2.

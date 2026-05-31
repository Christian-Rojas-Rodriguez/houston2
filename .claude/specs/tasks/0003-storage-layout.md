---
task: "0003"
slug: storage-layout
granularity: slice
version: 0.1.1
status: ready
declares:
  - type: policy
    name: storage-layout
    path: supabase/migrations/
scope:
  - supabase/migrations/
  - scripts/seed-templates.sh
  - tests/unit/0003__storage-layout.test.sh
branch: feat/0003-storage-layout
base: develop
---

# Task 0003 — `storage-layout`

> Bucket privado `houston` en Supabase Storage, políticas RLS de Storage por prefijo, y seed del template `sales`. Define las object keys canónicas (relativas al bucket) `{org}/{group}/agents/{agent_id}/`, `{org}/{grupo_general}/`, y `templates/`. Es la mitad-Storage de la invariante de aislamiento (la otra mitad es la RLS de Postgres de 0002).

> **Reapertura (2026-05-31) — fix R-STORAGE.** La policy `houston_tenant_isolation` original no matcheaba ningún objeto real (asumía un `name` con prefijo literal `houston/` y un segmento literal `general` que ningún writer produce). Esta versión reconcilia la policy + el test + esta spec con la convención de nombres **implementada** (relativa al bucket, grupo general por UUID). Ver "Reconciliación R-STORAGE". **Bump propuesto: `0.2.0` (minor)** — cambia el contrato documentado del layout de paths; pendiente del Auditor al merge.

## What

Una migración SQL de Supabase (en `supabase/migrations/`) + un script de seed + un test harness que, al completarse, garantizan:

1. **Un único bucket privado `houston`** (`public = false`) en Supabase Storage. Todo el acceso se gobierna por RLS — nunca por URL pública.
2. **Políticas RLS sobre `storage.objects`** que reproducen, en la capa de Storage, la misma invariante de aislamiento tenant que la Task 0002 aplica en Postgres:
   - Object key (relativa al bucket) `{org_id}/{group_id}/...` (el bucket `houston` se especifica fuera del `name`; el `name` almacenado **NO** incluye el prefijo `houston/`): visible/escribible solo si `org_segment = mine AND (group_segment = my_group OR group_segment es un grupo is_general de mi org OR role = 'org:owner')`.
   - Prefijo `templates/...`: **read-only** para todo usuario `authenticated` (SELECT); ninguna escritura desde `authenticated` (solo `service_role`).
3. **Seed idempotente** `scripts/seed-templates.sh` que sube `templates/sales/CLAUDE.md` (un CLAUDE.md mínimo) al bucket `houston`, usando `service_role`, funcionando contra el stack local (Colima) y contra cloud.

Topología canónica: **un solo bucket `houston`**. La URL REST `/storage/v1/object/houston/{org}/{group}/...` produce `bucket_id='houston'` y un `name` **relativo al bucket** = `{org}/{group}/...` (el primer segmento `houston` de la URL es el *bucket*, NO parte del `name`). `storage.foldername(name)` devuelve los segmentos de carpeta del `name` (sin el filename):

```
URL: /storage/v1/object/houston/{org}/{group}/agents/{agent}/CLAUDE.md
  → bucket_id='houston', name='{org}/{group}/agents/{agent}/CLAUDE.md'
  → foldername(name) = {org, group, agents, agent}          [1]=org [2]=group
name='{org}/{generalGroupUUID}/notes.md'                    [1]=org [2]=group (is_general=true)
name='templates/sales/CLAUDE.md'                            [1]=templates
```

> El grupo general se referencia por su **UUID** (los writers usan `group_id`, nunca el literal `general`); la visibilidad del grupo general se resuelve vía `is_general` en una subconsulta a `groups`, espejando la policy de `agents` de 0002.

Un usuario `authenticated` (sesión de Task 0004) puede listar/descargar objetos cuyo `name` empieza con `{su_org}/{su_group}/` o `{su_org}/{grupo_general}/`, recibe **cero** objetos de cualquier otra org/grupo, y puede **leer pero no escribir** `templates/`.

## Why

- **PRD §5 (P0):** "RLS habilitado en todas las tablas tenant-scoped **y en todos los buckets de Storage**." Storage está explícitamente en el contrato del MVP.
- **Leak test (PRD §3 / Task 0011):** la north-star es que ningún objeto de Storage de Org B sea retornado a un usuario de Org A. Esta task es la mitad-Storage de esa garantía; sin ella el leak test no puede pasar (y con la policy rota el leak test pasaría **vacuo** — ver Reconciliación R-STORAGE).
- **RFC §4.4:** prescribe el layout (`houston/{org_id}/{group_id}/agents/{agent_id}/`, `houston/{org_id}/general/`, `templates/`) y "Storage RLS: misma lógica `org = mine AND group ∈ {general, mine}` en prefijos de bucket. `templates/` es read-only para todos los usuarios autenticados."
- **Defensa en profundidad (RFC §1):** los Go handlers (0007/0008) usan el JWT del usuario contra la Storage API; la RLS de Storage es la última línea que evita la fuga. No negociable.
- **Desbloquea 0007/0008:** create-agent copia `templates/{id}/* → {org}/{group}/agents/{id}/` y run-agent descarga ese prefijo. Sin el bucket + policies correctas, esas operaciones fallan o filtran.

## How

### Archivos a crear

| Archivo | Acción |
|---|---|
| `supabase/migrations/<ts>_storage-layout.sql` | Bucket `houston` + policies RLS sobre `storage.objects` (migración inicial) |
| `supabase/migrations/20260531000001_storage-layout-fix.sql` | **Fix R-STORAGE**: `DROP`+`CREATE` de `houston_tenant_isolation` con índices relativos al bucket |
| `scripts/seed-templates.sh` | Subida idempotente de `templates/sales/CLAUDE.md` |
| `tests/unit/0003__storage-layout.test.sh` | Harness: assertions psql (RLS) + verificación del seed vía Storage API |

> El timestamp `<ts>` debe ordenar **después** de todas las migraciones existentes en `supabase/migrations/`. La migración inicial es `20260531000000_storage-layout.sql`; el fix es `20260531000001_storage-layout-fix.sql`.

### Migración — bucket + RLS (estado corregido)

```sql
-- 1. Bucket privado houston (idempotente)
INSERT INTO storage.buckets (id, name, public)
VALUES ('houston', 'houston', false)
ON CONFLICT (id) DO NOTHING;

-- storage.objects ya tiene RLS habilitada por defecto en Supabase.

-- 2. Aislamiento tenant (FOR ALL = SELECT+INSERT+UPDATE+DELETE).
--    SEGURIDAD: segmentos del path comparados COMO TEXTO contra current_tenant()
--    casteado a texto. NUNCA se castea el segmento a ::uuid (falla-seguro: un
--    segmento NULL/ajeno no matchea en vez de tirar error).
--    Nombres RELATIVOS al bucket: name='{org}/{group}/...' ([1]=org, [2]=group).
--    Visibilidad de grupo espeja la policy de `agents` de 0002 (own + is_general
--    + owner). WITH CHECK explícito (idéntico al USING) porque el path de subida
--    es parte del bug corregido (R-STORAGE).
CREATE POLICY houston_tenant_isolation ON storage.objects FOR ALL
TO authenticated
USING (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = (SELECT org_id::text FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR (storage.foldername(name))[2] IN (
      SELECT g.id::text FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
    )
  )
)
WITH CHECK (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = (SELECT org_id::text FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR (storage.foldername(name))[2] IN (
      SELECT g.id::text FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
    )
  )
);

-- 3. templates/ read-only para todo authenticated (solo SELECT). SIN CAMBIOS.
CREATE POLICY templates_readonly ON storage.objects FOR SELECT
TO authenticated
USING (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = 'templates'
);
```

Notas de diseño:

- **`bucket_id` es TEXT** (no uuid). Se usa el literal `'houston'`.
- **`WITH CHECK` explícito (post R-STORAGE):** la policy declara un `WITH CHECK` idéntico al `USING` porque el path de **subida** (create-agent con JWT de usuario) es parte del bug corregido. Un INSERT cross-tenant en `{otra_org}/...` es rechazado por el `[1] = mi org` del `WITH CHECK` (cubre AC-8).
- **templates read-only**: la policy tenant exige `[1] = mi org_id`; los objetos `templates/...` (`[1]='templates'` ≠ ningún UUID de org) no caen bajo ninguna policy de escritura para `authenticated` → INSERT/UPDATE/DELETE denegados. `templates_readonly` solo agrega SELECT. `service_role` bypasea RLS para el seed.
- **org:owner:** ve/escribe objetos de **todos los grupos de su org** (rama `role='org:owner'`), espejando la policy de `agents` de 0002. NO espeja `runs`.
- **`general` (decisión reconciliada — R-STORAGE):** el grupo general es una fila real de `groups` con `is_general=true` y su propio UUID; los writers (0007/0010) escriben el **UUID del grupo** en el path, nunca el literal `'general'`. La visibilidad general se resuelve vía subconsulta a `groups` usando `is_general`.

### Reconciliación R-STORAGE (migración append-only `20260531000001_storage-layout-fix.sql`)

La policy original (`20260531000000`) asumía un `name` con prefijo literal `houston/` (segmento `[1]`) y un segmento literal `general`. Pero Supabase almacena `storage.objects.name` **relativo al bucket**, y todos los writers (`internal/handlers/supabase_agent_store.go:51`, `tests/helpers/fixture.go:456`, `scripts/seed.go:272`) escriben `{org}/{group}/agents/{id}/...` con el **UUID** del grupo. Resultado: la policy no matcheaba ningún objeto real → usuarios autenticados veían **cero** objetos y los uploads con JWT de usuario eran rechazados.

La corrección es una migración forward **append-only** que hace `DROP POLICY IF EXISTS houston_tenant_isolation; CREATE POLICY ...` con los índices corregidos (`[1]=org`, `[2]=group`) y visibilidad general vía `is_general`. **No se modifican los writers** (0007/0010); se alinea la policy a la convención implementada. `templates_readonly` queda **sin cambios**. La migración original `20260531000000` **NO** se edita (append-only). El test `0003__storage-layout.test.sh` se actualiza para sembrar nombres relativos al bucket (la fixture anterior sembraba nombres `houston/...` ficticios que ningún writer produce → falso verde).

### Seed — `scripts/seed-templates.sh`

Idempotente, corre como `service_role` (bypasea RLS), autodetecta local vs cloud, sube al bucket `houston` la key `templates/sales/CLAUDE.md` vía la Storage REST API (`curl`):

```bash
#!/usr/bin/env bash
set -euo pipefail
SUPABASE_URL="${SUPABASE_URL:-http://127.0.0.1:54321}"
SERVICE_KEY="${SUPABASE_SERVICE_ROLE_KEY:?set SUPABASE_SERVICE_ROLE_KEY}"
BUCKET="houston"
KEY="templates/sales/CLAUDE.md"
BODY=$'# Sales Agent Template\n\nStub mínimo para Task 0003.\n'
code=$(curl -s -o /dev/null -w '%{http_code}' \
  -X POST "${SUPABASE_URL}/storage/v1/object/${BUCKET}/${KEY}" \
  -H "Authorization: Bearer ${SERVICE_KEY}" \
  -H "Content-Type: text/markdown" \
  -H "x-upsert: true" \
  --data-binary "${BODY}")
[[ "$code" == "200" || "$code" == "201" ]] || { echo "ERROR upload HTTP $code" >&2; exit 1; }
echo "seeded ${BUCKET}/${KEY} (HTTP $code)"
```

`x-upsert: true` lo hace idempotente.

### Test harness — `tests/unit/0003__storage-layout.test.sh`

Script Bash que corre contra el stack local (Colima up; DB vía `docker exec ... psql`; Storage API `http://127.0.0.1:54321`):

1. **Pre-condición:** `current_tenant()` existe (`pg_proc`).
2. **Assertions RLS (pgTAP, vía `docker exec ... psql`)** — insertar objetos fixture en `storage.objects` como `postgres` (bypasea RLS) con nombres **relativos al bucket** (`{org}/{group}/...`, general por UUID); luego `SET LOCAL ROLE authenticated` + `set_config('request.jwt.claims', ...)` para simular usuarios; verificar visibilidad.
3. **Verificación del seed (Storage API):** correr `scripts/seed-templates.sh` y confirmar la fila vía psql; correrlo dos veces → exit 0 (idempotencia).

### Dependencias

- **Task 0001** (hard): `current_tenant()` y la tabla `groups`/`is_general`.
- Independiente de Task 0002 — ambas comparten `current_tenant()`.

## Acceptance criteria

1. **AC-1 — Migración aplica + bucket privado:** tras `supabase db reset` (local) / `supabase db push` (cloud) sin errores, `SELECT id, public FROM storage.buckets WHERE id='houston'` retorna exactamente 1 fila con `public = false`.
2. **AC-2 — Policies existen:** `SELECT count(*) FROM pg_policies WHERE schemaname='storage' AND tablename='objects' AND policyname IN ('houston_tenant_isolation','templates_readonly')` retorna 2.
3. **AC-3 — Aislamiento por org:** un `group:member` de `org1/group1` ejecutando `SELECT name FROM storage.objects WHERE bucket_id='houston'` ve **0** objetos cuyo `name` empieza con `<org2>/`.
4. **AC-4 — Aislamiento por grupo:** el mismo `group:member` ve **0** objetos en `<org1>/<group2>/` (grupo ajeno, no general), y **sí** ve los de `<org1>/<group1>/` y `<org1>/<grupo_general_uuid>/`.
5. **AC-5 — org:owner ve todos los grupos de su org:** un `org:owner` de `org1` ve objetos de `<org1>/<group1>/` **y** `<org1>/<group2>/`, pero **0** de `<org2>/`.
6. **AC-6 — templates legible por cualquier autenticado:** tras el seed, cualquier sesión `authenticated` ve la fila `templates/sales/CLAUDE.md` (`SELECT ... WHERE name LIKE 'templates/%'` retorna ≥1).
7. **AC-7 — templates no escribible por authenticated:** un INSERT en `storage.objects` con `name` bajo `templates/...` desde rol `authenticated` es rechazado por RLS (SQLSTATE 42501).
8. **AC-8 — escritura cross-tenant denegada:** un INSERT con `name` `<org2>/...` desde una sesión de `org1` es rechazado por RLS (`WITH CHECK`, SQLSTATE 42501).
9. **AC-9 — seed sube el template:** tras correr `scripts/seed-templates.sh` contra el local, la fila `bucket_id='houston' AND name='templates/sales/CLAUDE.md'` existe en `storage.objects`.
10. **AC-10 — seed idempotente:** correr `scripts/seed-templates.sh` dos veces seguidas sale 0 en ambas.
11. **AC-11 — path corto falla-seguro:** un objeto con menos de 2 segmentos (p.ej. `name='<org1>'`, sin segmento de grupo) no es visible para `authenticated` (segmento `[2]` NULL no matchea).
12. **AC-12 — pre-condición:** `SELECT count(*) FROM pg_proc WHERE proname='current_tenant'` retorna 1 antes de evaluar cualquier policy de Storage.

## Out of scope

- Contenido real de `templates/sales/CLAUDE.md` más allá de un stub mínimo (decisión de producto).
- Copiar templates a prefijos de agente (`templates/{id}/* → {org}/{group}/agents/{id}/`) → Task 0007.
- Provisión de Supabase Auth / emisión de JWTs reales → Task 0004 (el harness simula claims con `set_config('request.jwt.claims', ...)`).
- Acceso a Storage desde el código Go → Tasks 0007/0008.
- El bug del **reader** `GetTemplate` (que lee del bucket `templates` en vez de `houston`) → Task 0007, **no** es parte de este fix.
- pgvector / RAG sobre objetos de Storage (post-MVP, RFC §3 non-goals).

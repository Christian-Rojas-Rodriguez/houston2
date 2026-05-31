---
task: "0003"
slug: storage-layout
granularity: slice
version: 0.1.0
status: draft
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

> Bucket privado `houston` en Supabase Storage, políticas RLS de Storage por prefijo, y seed del template `sales`. Define los prefijos canónicos `houston/{org}/{group}/agents/{agent_id}/`, `houston/{org}/general/`, y `templates/`. Es la mitad-Storage de la invariante de aislamiento (la otra mitad es la RLS de Postgres de 0002).

## What

Una migración SQL de Supabase (en `supabase/migrations/`) + un script de seed + un test harness que, al completarse, garantizan:

1. **Un único bucket privado `houston`** (`public = false`) en Supabase Storage. Todo el acceso se gobierna por RLS — nunca por URL pública.
2. **Políticas RLS sobre `storage.objects`** que reproducen, en la capa de Storage, la misma invariante de aislamiento tenant que la Task 0002 aplica en Postgres:
   - Prefijo `houston/{org_id}/{group_id}/...` y `houston/{org_id}/general/...`: visible/escribible solo si `org_id = mine AND ({group_id} = my_group OR el segmento es 'general' OR role = 'org:owner')`.
   - Prefijo `templates/...`: **read-only** para todo usuario `authenticated` (SELECT); ninguna escritura desde `authenticated` (solo `service_role`).
3. **Seed idempotente** `scripts/seed-templates.sh` que sube `templates/sales/CLAUDE.md` (un CLAUDE.md mínimo) al bucket `houston`, usando `service_role`, funcionando contra el stack local (Colima) y contra cloud.

Topología canónica (decidida en el gate): **un solo bucket `houston`**; las object keys son literalmente `houston/{org}/{group}/...` y `templates/...` (coincide palabra-por-palabra con RFC §4.4 y con los paths que 0007/0008 ya referencian). `storage.foldername(name)` devuelve los segmentos de carpeta (sin el filename):

```
houston/{org}/{group}/agents/{agent}/CLAUDE.md → {houston, org, group, agents, agent}   [1]=houston [2]=org [3]=group
houston/{org}/general/notes.md                 → {houston, org, general}                  [1]=houston [2]=org [3]=general
templates/sales/CLAUDE.md                      → {templates, sales}                       [1]=templates
```

Un usuario `authenticated` (sesión de Task 0004) puede listar/descargar objetos de `houston/{su_org}/{su_group}/` y `houston/{su_org}/general/`, recibe **cero** objetos en cualquier otro prefijo de org/grupo, y puede **leer pero no escribir** `templates/`.

## Why

- **PRD §5 (P0):** "RLS habilitado en todas las tablas tenant-scoped **y en todos los buckets de Storage**." Storage está explícitamente en el contrato del MVP.
- **Leak test (PRD §3 / Task 0011):** la north-star es que ningún objeto de Storage de Org B sea retornado a un usuario de Org A. Esta task es la mitad-Storage de esa garantía; sin ella el leak test no puede pasar.
- **RFC §4.4:** prescribe el layout (`houston/{org_id}/{group_id}/agents/{agent_id}/`, `houston/{org_id}/general/`, `templates/`) y "Storage RLS: misma lógica `org = mine AND group ∈ {general, mine}` en prefijos de bucket. `templates/` es read-only para todos los usuarios autenticados."
- **Defensa en profundidad (RFC §1):** los Go handlers (0007/0008) usan `service_role` y bypasean RLS; si tienen un bug de autorización, la RLS de Storage es la última línea que evita la fuga. No negociable.
- **Desbloquea 0007/0008:** create-agent copia `templates/{id}/* → houston/{org}/{group}/agents/{id}/` y run-agent descarga ese prefijo. Sin el bucket + policies, esas operaciones fallan o filtran.

## How

### Archivos a crear

| Archivo | Acción |
|---|---|
| `supabase/migrations/<ts>_storage-layout.sql` | Bucket `houston` + policies RLS sobre `storage.objects` |
| `scripts/seed-templates.sh` | Subida idempotente de `templates/sales/CLAUDE.md` |
| `tests/unit/0003__storage-layout.test.sh` | Harness: assertions psql (RLS) + verificación del seed vía Storage API |

> El timestamp `<ts>` debe ordenar **después** de todas las migraciones existentes en `supabase/migrations/` (chequear con `ls`; al momento de escribir existe `20260530000000_db-schema.sql`). Usar p.ej. `20260531000000_storage-layout.sql`.

### Migración — bucket + RLS

```sql
-- 1. Bucket privado houston (idempotente)
INSERT INTO storage.buckets (id, name, public)
VALUES ('houston', 'houston', false)
ON CONFLICT (id) DO NOTHING;

-- storage.objects ya tiene RLS habilitada por defecto en Supabase
-- (verificar: SELECT relrowsecurity FROM pg_class
--  WHERE relname='objects' AND relnamespace='storage'::regnamespace).

-- 2. Aislamiento tenant sobre el prefijo houston/  (FOR ALL = SELECT+INSERT+UPDATE+DELETE)
--    SEGURIDAD: se comparan los segmentos del path COMO TEXTO contra los valores
--    confiables de current_tenant() casteados a texto. NO se castea el segmento
--    del path a ::uuid (un segmento mal formado tiraría error en vez de ocultar la
--    fila). Comparar como texto es falla-seguro: segmento NULL/ajeno → no matchea.
CREATE POLICY houston_tenant_isolation ON storage.objects FOR ALL
TO authenticated
USING (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = 'houston'
  AND (storage.foldername(name))[2] = (SELECT org_id::text FROM current_tenant())
  AND (
    (storage.foldername(name))[3] = 'general'
    OR (storage.foldername(name))[3] = (SELECT group_id::text FROM current_tenant())
    OR (SELECT role FROM current_tenant()) = 'org:owner'
  )
);

-- 3. templates/ read-only para todo authenticated (solo SELECT; sin INSERT/UPDATE/DELETE)
CREATE POLICY templates_readonly ON storage.objects FOR SELECT
TO authenticated
USING (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = 'templates'
);
```

Notas de diseño:

- **`bucket_id` es TEXT** (no uuid). Se usa el literal `'houston'`. (Verificar el tipo real contra el schema `storage` vivo durante implementación.)
- **`FOR ALL` con solo `USING`**: en Postgres, si se omite `WITH CHECK`, la expresión `USING` se usa también como `WITH CHECK` para INSERT/UPDATE → un INSERT cross-tenant en `houston/{otra_org}/...` es rechazado (cubre el AC-8). Mismo patrón validado en 0002.
- **templates read-only**: como la policy tenant exige `[1]='houston'`, los objetos `templates/...` ([1]='templates') no caen bajo ninguna policy de escritura para `authenticated` → INSERT/UPDATE/DELETE denegados. `templates_readonly` solo agrega SELECT. `service_role` bypasea RLS para el seed.
- **org:owner (decisión defaulteada en el gate):** ve/escribe objetos de **todos los grupos de su org** (rama `role='org:owner'`), espejando la policy de `agents` de 0002 (Storage guarda archivos de agentes). NO espeja `runs` (que sí restringe al owner).
- **`general` (decisión defaulteada):** es el literal de string `'general'` en el segmento [3] del path, no el UUID del grupo general.

### Seed — `scripts/seed-templates.sh`

Idempotente, corre como `service_role` (bypasea RLS), autodetecta local vs cloud, sube al bucket `houston` la key `templates/sales/CLAUDE.md` vía la Storage REST API (`curl`, sin depender de `supabase storage cp`):

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

`x-upsert: true` lo hace idempotente. La key local del `service_role` se obtiene de `supabase status` / la salida de `supabase start`. (Si durante implementación `supabase storage cp` resulta estable en la CLI v2.102.0, puede reemplazar al curl — pero curl es el fallback portable.)

### Test harness — `tests/unit/0003__storage-layout.test.sh`

Script Bash (formato `.sh` declarado) que corre contra el stack local (Colima up; DB `postgresql://postgres:postgres@127.0.0.1:54322/postgres`; Storage API `http://127.0.0.1:54321`):

1. **Pre-condición:** `current_tenant()` existe (`pg_proc`).
2. **Assertions RLS (pgTAP, vía `docker exec ... psql`)** — patrón idéntico a 0002: insertar objetos fixture en `storage.objects` como `postgres`/`service_role` (bypasea RLS) en varios prefijos; luego `SET LOCAL ROLE authenticated` + `set_config('request.jwt.claims', ...)` para simular usuarios; verificar visibilidad. `CREATE EXTENSION IF NOT EXISTS pgtap;` dentro de la transacción (auto-contenido, a diferencia del test de 0001).
3. **Verificación del seed (Storage API):** correr `scripts/seed-templates.sh` y `GET /storage/v1/object/...` con un JWT autenticado → 200 + body no vacío; correrlo dos veces → exit 0 (idempotencia).

### Dependencias

- **Task 0001** (hard): `current_tenant()` y la tabla `groups`/`is_general`.
- Independiente de Task 0002 (esa es Postgres-RLS; esta es Storage-RLS) — ambas comparten `current_tenant()`.

## Acceptance criteria

1. **AC-1 — Migración aplica + bucket privado:** tras `supabase db reset` (local) / `supabase db push` (cloud) sin errores, `SELECT id, public FROM storage.buckets WHERE id='houston'` retorna exactamente 1 fila con `public = false`.
2. **AC-2 — Policies existen:** `SELECT count(*) FROM pg_policies WHERE schemaname='storage' AND tablename='objects' AND policyname IN ('houston_tenant_isolation','templates_readonly')` retorna 2.
3. **AC-3 — Aislamiento por org:** un `group:member` de `org1/group1` ejecutando `SELECT name FROM storage.objects WHERE bucket_id='houston'` ve **0** objetos cuyo path empieza con `houston/<org2>/`.
4. **AC-4 — Aislamiento por grupo:** el mismo `group:member` ve **0** objetos en `houston/<org1>/<group2>/` (grupo ajeno, no general), y **sí** ve los de `houston/<org1>/<group1>/` y `houston/<org1>/general/`.
5. **AC-5 — org:owner ve todos los grupos de su org:** un `org:owner` de `org1` ve objetos de `houston/<org1>/<group1>/` **y** `houston/<org1>/<group2>/`, pero **0** de `houston/<org2>/`.
6. **AC-6 — templates legible por cualquier autenticado:** tras el seed, cualquier sesión `authenticated` ve la fila `templates/sales/CLAUDE.md` (`SELECT ... WHERE name LIKE 'templates/%'` retorna ≥1).
7. **AC-7 — templates no escribible por authenticated:** un INSERT en `storage.objects` con `name` bajo `templates/...` desde rol `authenticated` es rechazado por RLS (policy violation / 0 filas).
8. **AC-8 — escritura cross-tenant denegada:** un INSERT con path `houston/<org2>/...` desde una sesión de `org1` es rechazado por RLS (`WITH CHECK` derivado del `USING`).
9. **AC-9 — seed sube el template:** tras correr `scripts/seed-templates.sh` contra el local, `GET /storage/v1/object/houston/templates/sales/CLAUDE.md` con un JWT autenticado retorna HTTP 200 y body no vacío.
10. **AC-10 — seed idempotente:** correr `scripts/seed-templates.sh` dos veces seguidas sale 0 en ambas.
11. **AC-11 — path corto falla-seguro:** un objeto con menos de 3 segmentos (p.ej. `houston/<org1>`) no es visible para `authenticated` (la comparación de segmento NULL no matchea).
12. **AC-12 — pre-condición:** `SELECT count(*) FROM pg_proc WHERE proname='current_tenant'` retorna 1 antes de evaluar cualquier policy de Storage.

## Out of scope

- Contenido real de `templates/sales/CLAUDE.md` más allá de un stub mínimo (decisión de producto).
- Copiar templates a prefijos de agente (`templates/{id}/* → houston/{org}/{group}/agents/{id}/`) → Task 0007.
- Provisión de Supabase Auth / emisión de JWTs reales → Task 0004 (el harness simula claims con `set_config('request.jwt.claims', ...)`).
- Acceso a Storage desde el código Go → Tasks 0007/0008.
- `WITH CHECK` explícito separado del `USING` (post-MVP hardening, igual que 0002).
- pgvector / RAG sobre objetos de Storage (post-MVP, RFC §3 non-goals).

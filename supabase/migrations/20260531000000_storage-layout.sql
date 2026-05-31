-- Migration: 20260531000000_storage-layout
-- Task 0003 — storage-layout
-- Creates the private bucket 'houston' and Storage RLS policies for
-- tenant isolation and read-only template access.
--
-- Spec: .claude/specs/tasks/0003-storage-layout.md

-- 1. Bucket privado houston (idempotente)
INSERT INTO storage.buckets (id, name, public)
VALUES ('houston', 'houston', false)
ON CONFLICT (id) DO NOTHING;

-- storage.objects ya tiene RLS habilitada por defecto en Supabase.
-- (SELECT relrowsecurity FROM pg_class
--  WHERE relname='objects' AND relnamespace='storage'::regnamespace) => true

-- 2. Aislamiento tenant sobre el prefijo houston/ (FOR ALL = SELECT+INSERT+UPDATE+DELETE)
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

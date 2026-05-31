-- Migration: 20260531000001_storage-layout-fix
-- Task 0003 — storage-layout (R-STORAGE fix)
--
-- Reconciles houston_tenant_isolation with the IMPLEMENTED object-naming
-- convention. Supabase stores storage.objects.name BUCKET-RELATIVE: a REST
-- upload to /storage/v1/object/houston/<org>/<group>/agents/<id>/CLAUDE.md
-- yields bucket_id='houston', name='<org>/<group>/agents/<id>/CLAUDE.md'
-- (NO leading 'houston/'; that first URL segment is the BUCKET). The original
-- policy (20260531000000) required a literal 'houston/' first segment and a
-- literal 'general' group segment, so it matched ZERO real objects for
-- authenticated users and rejected real user-JWT uploads.
--
-- This migration drops and recreates ONLY houston_tenant_isolation:
--   [1] = org_id::text          (was the literal 'houston'; [2] was org)
--   [2] = group id::text        (was [3])
-- and resolves general-group / own-group / owner visibility via a `groups`
-- subquery using is_general — mirroring the agents policy in
-- 20260530000001_rls-postgres.sql (the table-level source of truth).
--
-- templates_readonly is intentionally LEFT UNCHANGED: templates live under
-- name='templates/...' (bucket-relative), so its [1]='templates' check is
-- already correct.
--
-- SECURITY: path segments are compared AS TEXT against trusted current_tenant()
-- values cast to text; a segment is NEVER cast to ::uuid (a malformed segment
-- would raise instead of hiding the row). Text comparison is fail-safe: a NULL
-- or foreign segment simply does not match (AC-11: short paths → [2] IS NULL).
--
-- WITH CHECK is made EXPLICIT (identical to USING) because the create-agent
-- upload path (user JWT) is part of the bug this migration fixes.

DROP POLICY IF EXISTS houston_tenant_isolation ON storage.objects;

CREATE POLICY houston_tenant_isolation ON storage.objects FOR ALL
TO authenticated
USING (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = (SELECT org_id::text FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR (storage.foldername(name))[2] IN (
      SELECT g.id::text
      FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (
          g.id = (SELECT group_id FROM current_tenant())
          OR g.is_general = true
        )
    )
  )
)
WITH CHECK (
  bucket_id = 'houston'
  AND (storage.foldername(name))[1] = (SELECT org_id::text FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR (storage.foldername(name))[2] IN (
      SELECT g.id::text
      FROM groups g
      WHERE g.org_id = (SELECT org_id FROM current_tenant())
        AND (
          g.id = (SELECT group_id FROM current_tenant())
          OR g.is_general = true
        )
    )
  )
);

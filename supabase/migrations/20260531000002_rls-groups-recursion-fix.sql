-- Migration: 20260531000002_rls-groups-recursion-fix
-- Task 0002 — rls-postgres (corrective: groups-policy infinite recursion on cloud)
--
-- The recursion-proof (Option A) groups policy lives in
-- 20260530000001_rls-postgres.sql, but CLOUD had a stale RECURSIVE version
-- applied: a buggy version was pushed under that migration's `version` before
-- the fix landed, and Supabase never re-applies an already-applied migration.
-- The buggy policy had a self-referential `SELECT g.id FROM groups g ...`
-- subquery → "infinite recursion detected in policy for relation groups"
-- (SQLSTATE 42P17), breaking EVERY query that touches groups
-- (agents / memberships / runs / storage) on cloud.
--
-- This forward (append-only) migration DROPs + reCREATEs ONLY the groups policy
-- with the recursion-proof definition (references own-row columns only). Local
-- is already correct, so this is a no-op-equivalent locally and a real fix on
-- cloud after `supabase db push`. The other 5 table policies are identical
-- between the buggy and fixed eras (only groups changed), so they are untouched.

DROP POLICY IF EXISTS tenant_isolation ON groups;

CREATE POLICY tenant_isolation ON groups FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR id = (SELECT group_id FROM current_tenant())
    OR is_general = true
  )
);

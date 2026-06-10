-- Migration: 20260530000001_rls-postgres
-- Task 0002 — rls-postgres
-- Adds CREATE POLICY statements to all six tenant-scoped tables.
-- Task 0001 already enabled RLS + FORCE RLS and created current_tenant().
-- This migration only adds the policies — no DDL changes to tables or functions.
--
-- RFC §4.3 is the authoritative reference for these policy invariants.

-- 1. organizations — user sees only their own org
CREATE POLICY tenant_isolation ON organizations FOR ALL USING (
  id = (SELECT org_id FROM current_tenant())
);

-- 2. groups — org:owner sees all groups in their org;
--    other roles see only their assigned group + the general group
--
-- FIX (recursion-proof, Option A): The original policy had a self-referential
-- subquery `SELECT g.id FROM groups g WHERE ...` which caused infinite recursion
-- because evaluating the policy for groups triggered the same policy again.
-- The invariant can be expressed without any subquery on groups by referencing
-- only the current row's own columns (org_id, id, is_general), which breaks the cycle.
CREATE POLICY tenant_isolation ON groups FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (
    (SELECT role FROM current_tenant()) = 'org:owner'
    OR id = (SELECT group_id FROM current_tenant())
    OR is_general = true
  )
);

-- 3. memberships — same scoping pattern as groups
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

-- 4. agents — same scoping pattern as groups
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

-- 5. runs — strict group isolation; org:owner does NOT get cross-group visibility
--    (RFC §4.5: "solo las propias" runs — user_id restriction is middleware concern)
CREATE POLICY runs_isolation ON runs FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND group_id IN (
    SELECT g.id FROM groups g
    WHERE g.org_id = (SELECT org_id FROM current_tenant())
      AND (g.id = (SELECT group_id FROM current_tenant()) OR g.is_general = true)
  )
);

-- 6. org_credentials — only org:owner of the owning org can read/write
CREATE POLICY credentials_owner ON org_credentials FOR ALL USING (
  org_id = (SELECT org_id FROM current_tenant())
  AND (SELECT role FROM current_tenant()) = 'org:owner'
);

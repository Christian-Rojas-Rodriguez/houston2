-- pgTAP test suite for Task 0001 — db-schema
-- Run after: supabase start && supabase db reset
-- Runner:    pg_prove tests/unit/0001__db-schema.test.sql
--            or: psql -U postgres -f tests/unit/0001__db-schema.test.sql
--
-- Tests MUST FAIL before the migration is applied (no tables exist yet).
-- Each test maps to exactly one acceptance criterion from the spec.

BEGIN;

SELECT plan(35);

-- ============================================================
-- AC-1: supabase/migrations/ exists with exactly one .sql file
-- (This is a filesystem check — validated by the CI step that
--  runs the suite; here we test the migration's effects instead.)
-- Proxy: verify the migration was applied by checking pg_class
-- ============================================================

-- AC-3: organizations table exists with correct columns
SELECT has_table('public', 'organizations', 'AC-3: organizations table exists');
SELECT has_column('public', 'organizations', 'id', 'AC-3: organizations.id exists');
SELECT col_type_is('public', 'organizations', 'id', 'uuid', 'AC-3: organizations.id is uuid');
SELECT col_is_pk('public', 'organizations', 'id', 'AC-3: organizations.id is primary key');
SELECT has_column('public', 'organizations', 'name', 'AC-3: organizations.name exists');
SELECT col_type_is('public', 'organizations', 'name', 'text', 'AC-3: organizations.name is text');
SELECT col_not_null('public', 'organizations', 'name', 'AC-3: organizations.name is NOT NULL');
SELECT has_column('public', 'organizations', 'created_at', 'AC-3: organizations.created_at exists');
SELECT col_type_is('public', 'organizations', 'created_at', 'timestamp with time zone', 'AC-3: organizations.created_at is timestamptz');

-- AC-4: groups table exists with correct columns and constraints
SELECT has_table('public', 'groups', 'AC-4: groups table exists');
SELECT has_column('public', 'groups', 'org_id', 'AC-4: groups.org_id exists');
SELECT col_not_null('public', 'groups', 'org_id', 'AC-4: groups.org_id is NOT NULL');
SELECT col_not_null('public', 'groups', 'is_general', 'AC-4: groups.is_general is NOT NULL');
SELECT col_default_is(
  'public', 'groups', 'is_general', 'false',
  'AC-4: groups.is_general defaults to false'
);

-- AC-5: memberships table exists with correct columns and constraints
SELECT has_table('public', 'memberships', 'AC-5: memberships table exists');
SELECT has_column('public', 'memberships', 'user_id', 'AC-5: memberships.user_id exists');
SELECT col_not_null('public', 'memberships', 'user_id', 'AC-5: memberships.user_id is NOT NULL');
SELECT has_column('public', 'memberships', 'role', 'AC-5: memberships.role exists');
SELECT col_not_null('public', 'memberships', 'role', 'AC-5: memberships.role is NOT NULL');

-- AC-6: agents table exists with correct columns
SELECT has_table('public', 'agents', 'AC-6: agents table exists');
SELECT has_column('public', 'agents', 'source', 'AC-6: agents.source exists');
SELECT col_not_null('public', 'agents', 'source', 'AC-6: agents.source is NOT NULL');
SELECT col_type_is('public', 'agents', 'config', 'jsonb', 'AC-6: agents.config is jsonb');

-- AC-7: runs table exists with correct columns
SELECT has_table('public', 'runs', 'AC-7: runs table exists');
SELECT has_column('public', 'runs', 'prompt', 'AC-7: runs.prompt exists');
SELECT col_not_null('public', 'runs', 'prompt', 'AC-7: runs.prompt is NOT NULL');
SELECT col_default_is(
  'public', 'runs', 'status', '''pending''::text',
  'AC-7: runs.status defaults to pending'
);

-- AC-8: org_credentials table exists with correct columns
SELECT has_table('public', 'org_credentials', 'AC-8: org_credentials table exists');
SELECT has_column('public', 'org_credentials', 'anthropic_key', 'AC-8: org_credentials.anthropic_key exists');
SELECT col_not_null('public', 'org_credentials', 'anthropic_key', 'AC-8: org_credentials.anthropic_key is NOT NULL');

-- AC-9: anthropic_key is bytea (not text)
SELECT col_type_is(
  'public', 'org_credentials', 'anthropic_key', 'bytea',
  'AC-9: org_credentials.anthropic_key is bytea type'
);

-- AC-10: pgcrypto extension is installed
SELECT ok(
  (SELECT count(*) = 1 FROM pg_extension WHERE extname = 'pgcrypto'),
  'AC-10: pgcrypto extension is installed'
);

-- AC-11: RLS is enabled on all six tables
SELECT ok(
  (SELECT relrowsecurity FROM pg_class WHERE relname = 'organizations' AND relnamespace = 'public'::regnamespace),
  'AC-11: RLS enabled on organizations'
);
SELECT ok(
  (SELECT relrowsecurity FROM pg_class WHERE relname = 'groups' AND relnamespace = 'public'::regnamespace),
  'AC-11: RLS enabled on groups'
);
SELECT ok(
  (SELECT relrowsecurity FROM pg_class WHERE relname = 'memberships' AND relnamespace = 'public'::regnamespace),
  'AC-11: RLS enabled on memberships'
);
SELECT ok(
  (SELECT relrowsecurity FROM pg_class WHERE relname = 'agents' AND relnamespace = 'public'::regnamespace),
  'AC-11: RLS enabled on agents'
);
SELECT ok(
  (SELECT relrowsecurity FROM pg_class WHERE relname = 'runs' AND relnamespace = 'public'::regnamespace),
  'AC-11: RLS enabled on runs'
);
SELECT ok(
  (SELECT relrowsecurity FROM pg_class WHERE relname = 'org_credentials' AND relnamespace = 'public'::regnamespace),
  'AC-11: RLS enabled on org_credentials'
);

-- AC-12: current_tenant() function exists with SECURITY DEFINER
-- (pgTAP has_function checks existence; SECURITY DEFINER checked via pg_proc)
SELECT has_function(
  'public',
  'current_tenant',
  'AC-12: current_tenant() function exists in public schema'
);
SELECT ok(
  (SELECT prosecdef FROM pg_proc WHERE proname = 'current_tenant' AND pronamespace = 'public'::regnamespace),
  'AC-12: current_tenant() has SECURITY DEFINER'
);

-- AC-13: current_tenant() returns 0 rows when no membership exists
-- We test this by calling the function directly; with no data it must return empty.
SELECT is(
  (SELECT count(*)::int FROM current_tenant()),
  0,
  'AC-13: current_tenant() returns 0 rows when auth.uid() has no membership'
);

SELECT finish();
ROLLBACK;

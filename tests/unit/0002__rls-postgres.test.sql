-- pgTAP test suite for Task 0002 — rls-postgres
-- All 14 acceptance criteria from .claude/specs/tasks/0002-rls-postgres.md
--
-- Runner (CLOUD ONLY — no local stack):
--   supabase db query --linked < tests/unit/0002__rls-postgres.test.sql
--
-- This suite MUST FAIL before migration 20260530000001_rls-postgres.sql is applied
-- (no policies exist yet — catalog checks for policy names will return 0 rows).
--
-- RLS enforcement tests (AC-3 to AC-12) work by:
--   1. Inserting seed data as postgres (bypasses RLS).
--   2. SET LOCAL ROLE authenticated + SET LOCAL request.jwt.claims to simulate a user.
--   3. Querying tables — now RLS applies.
--   4. ROLLBACK at the end — no data persists.
--
-- pgTAP must be installed; we do it inside the transaction so it is
-- rolled back with everything else if something goes wrong.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgtap;

-- ============================================================
-- Seed data (inserted as postgres superuser, bypasses RLS)
-- All inserts happen BEFORE any SET LOCAL ROLE.
-- ============================================================

-- Two organizations
INSERT INTO public.organizations (id, name) VALUES
  ('00000000-0000-0000-0000-000000000001'::uuid, 'Org A'),
  ('00000000-0000-0000-0000-000000000002'::uuid, 'Org B');

-- Groups for Org A: one general, one specific
INSERT INTO public.groups (id, org_id, name, is_general) VALUES
  ('00000000-0000-0000-0001-000000000001'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 'general', true),
  ('00000000-0000-0000-0001-000000000002'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 'team-x', false),
  ('00000000-0000-0000-0001-000000000003'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 'team-y', false);

-- Groups for Org B
INSERT INTO public.groups (id, org_id, name, is_general) VALUES
  ('00000000-0000-0000-0002-000000000001'::uuid, '00000000-0000-0000-0000-000000000002'::uuid, 'general-b', true);

-- Auth users (insert into auth.users as superuser)
-- user_member_a: group:member of Org A / team-x
-- user_owner_a: org:owner of Org A / general
-- user_member_b: group:member of Org B / general-b
-- user_no_membership: authenticated but no membership
-- user_manager_a: group:manager of Org A / team-x
INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at, created_at, updated_at, aud, role)
VALUES
  ('aaaaaaaa-0000-0000-0000-000000000001'::uuid, 'member-a@test.local', 'x', now(), now(), now(), 'authenticated', 'authenticated'),
  ('aaaaaaaa-0000-0000-0000-000000000002'::uuid, 'owner-a@test.local',  'x', now(), now(), now(), 'authenticated', 'authenticated'),
  ('aaaaaaaa-0000-0000-0000-000000000003'::uuid, 'member-b@test.local', 'x', now(), now(), now(), 'authenticated', 'authenticated'),
  ('aaaaaaaa-0000-0000-0000-000000000004'::uuid, 'no-member@test.local','x', now(), now(), now(), 'authenticated', 'authenticated'),
  ('aaaaaaaa-0000-0000-0000-000000000005'::uuid, 'manager-a@test.local','x', now(), now(), now(), 'authenticated', 'authenticated');

-- Memberships
INSERT INTO public.memberships (id, user_id, org_id, group_id, role) VALUES
  ('bbbbbbbb-0000-0000-0000-000000000001'::uuid,
   'aaaaaaaa-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000002'::uuid,
   'group:member'),
  ('bbbbbbbb-0000-0000-0000-000000000002'::uuid,
   'aaaaaaaa-0000-0000-0000-000000000002'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000001'::uuid,
   'org:owner'),
  ('bbbbbbbb-0000-0000-0000-000000000003'::uuid,
   'aaaaaaaa-0000-0000-0000-000000000003'::uuid,
   '00000000-0000-0000-0000-000000000002'::uuid,
   '00000000-0000-0000-0002-000000000001'::uuid,
   'group:member'),
  ('bbbbbbbb-0000-0000-0000-000000000005'::uuid,
   'aaaaaaaa-0000-0000-0000-000000000005'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000002'::uuid,
   'group:manager');

-- Agents: one in team-x (Org A), one in team-y (Org A)
INSERT INTO public.agents (id, org_id, group_id, name, source, config) VALUES
  ('cccccccc-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000002'::uuid,
   'agent-x', 'blank', NULL),
  ('cccccccc-0000-0000-0000-000000000002'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000003'::uuid,
   'agent-y', 'blank', NULL);

-- Runs: one by user_member_a in team-x, one in team-y (by owner, for AC-8)
INSERT INTO public.runs (id, agent_id, org_id, group_id, user_id, prompt, status) VALUES
  ('dddddddd-0000-0000-0000-000000000001'::uuid,
   'cccccccc-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000002'::uuid,
   'aaaaaaaa-0000-0000-0000-000000000001'::uuid,
   'run by member-a in team-x', 'pending'),
  ('dddddddd-0000-0000-0000-000000000002'::uuid,
   'cccccccc-0000-0000-0000-000000000002'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0001-000000000003'::uuid,
   'aaaaaaaa-0000-0000-0000-000000000002'::uuid,
   'run in team-y (by owner)', 'pending');

-- org_credentials for both orgs
INSERT INTO public.org_credentials (id, org_id, anthropic_key) VALUES
  ('eeeeeeee-0000-0000-0000-000000000001'::uuid,
   '00000000-0000-0000-0000-000000000001'::uuid,
   'dummy-key-a'),
  ('eeeeeeee-0000-0000-0000-000000000002'::uuid,
   '00000000-0000-0000-0000-000000000002'::uuid,
   'dummy-key-b');

-- ============================================================
-- Plan: 14 ACs with assertions per AC
-- ============================================================

SELECT plan(24);

-- ============================================================
-- AC-1 & AC-2: Policies exist in pg_policies catalog
-- Fails before migration: no rows for these policy names
-- ============================================================

SELECT is(
  (SELECT count(*)::int
   FROM pg_policies
   WHERE schemaname = 'public'
     AND tablename IN ('organizations','groups','memberships','agents','runs','org_credentials')),
  6,
  'AC-1/AC-2: exactly 6 policies exist across the six tenant-scoped tables'
);

SELECT ok(
  EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='organizations'   AND policyname='tenant_isolation'),
  'AC-2: tenant_isolation policy exists on organizations'
);
SELECT ok(
  EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='groups'         AND policyname='tenant_isolation'),
  'AC-2: tenant_isolation policy exists on groups'
);
SELECT ok(
  EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='memberships'    AND policyname='tenant_isolation'),
  'AC-2: tenant_isolation policy exists on memberships'
);
SELECT ok(
  EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='agents'         AND policyname='tenant_isolation'),
  'AC-2: tenant_isolation policy exists on agents'
);
SELECT ok(
  EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='runs'           AND policyname='runs_isolation'),
  'AC-2: runs_isolation policy exists on runs'
);
SELECT ok(
  EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='org_credentials' AND policyname='credentials_owner'),
  'AC-2: credentials_owner policy exists on org_credentials'
);

-- ============================================================
-- AC-3: user with no membership sees 0 rows on all six tables
-- (uid: aaaaaaaa-0000-0000-0000-000000000004)
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000004","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.organizations),
  0,
  'AC-3: no-membership user sees 0 rows in organizations'
);
SELECT is(
  (SELECT count(*)::int FROM public.groups),
  0,
  'AC-3: no-membership user sees 0 rows in groups'
);
SELECT is(
  (SELECT count(*)::int FROM public.org_credentials),
  0,
  'AC-3: no-membership user sees 0 rows in org_credentials'
);

RESET ROLE;

-- ============================================================
-- AC-4: user_member_a (Org A) sees only Org A in organizations
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000001","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.organizations),
  1,
  'AC-4: member-a sees exactly 1 organization'
);
SELECT is(
  (SELECT id FROM public.organizations LIMIT 1),
  '00000000-0000-0000-0000-000000000001'::uuid,
  'AC-4: member-a sees only Org A'
);

RESET ROLE;

-- ============================================================
-- AC-5: group:member (team-x) sees team-x + general, NOT team-y
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000001","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.groups
   WHERE org_id = '00000000-0000-0000-0000-000000000001'::uuid),
  2,
  'AC-5: group:member sees 2 groups (own + general), not team-y'
);
SELECT ok(
  NOT EXISTS (
    SELECT 1 FROM public.groups
    WHERE id = '00000000-0000-0000-0001-000000000003'::uuid
  ),
  'AC-5: group:member cannot see team-y'
);

RESET ROLE;

-- ============================================================
-- AC-6: org:owner (owner-a) sees all 3 groups in Org A
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000002","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.groups
   WHERE org_id = '00000000-0000-0000-0000-000000000001'::uuid),
  3,
  'AC-6: org:owner sees all 3 groups in their org'
);

RESET ROLE;

-- ============================================================
-- AC-7: runs policy — group:member (team-x) sees run in team-x,
-- not run in team-y (group isolation, regardless of user_id)
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000001","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.runs
   WHERE org_id = '00000000-0000-0000-0000-000000000001'::uuid),
  1,
  'AC-7: group:member sees only runs in their group (team-x), not team-y'
);

RESET ROLE;

-- ============================================================
-- AC-8: org:owner is in general group (not team-y directly);
-- owner cannot see runs in team-y via policy SQL
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000002","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.runs
   WHERE group_id = '00000000-0000-0000-0001-000000000003'::uuid),
  0,
  'AC-8: org:owner (in general group only) cannot see runs in team-y'
);

RESET ROLE;

-- ============================================================
-- AC-9: group:member and group:manager cannot read org_credentials
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000001","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.org_credentials),
  0,
  'AC-9: group:member sees 0 rows in org_credentials'
);

RESET ROLE;

SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000005","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.org_credentials),
  0,
  'AC-9: group:manager sees 0 rows in org_credentials'
);

RESET ROLE;

-- ============================================================
-- AC-10: org:owner can read their own org_credentials
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000002","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.org_credentials
   WHERE org_id = '00000000-0000-0000-0000-000000000001'::uuid),
  1,
  'AC-10: org:owner can read org_credentials for their own org'
);

RESET ROLE;

-- ============================================================
-- AC-11: org:owner cannot read org_credentials of another org
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000002","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.org_credentials
   WHERE org_id = '00000000-0000-0000-0000-000000000002'::uuid),
  0,
  'AC-11: org:owner cannot read org_credentials of Org B'
);

RESET ROLE;

-- ============================================================
-- AC-12: INSERT into foreign org blocked by RLS
-- group:member of Org A tries to insert agent with org_id = Org B
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000001","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT throws_ok(
  $$INSERT INTO public.agents (org_id, group_id, name, source)
    VALUES (
      '00000000-0000-0000-0000-000000000002'::uuid,
      '00000000-0000-0000-0002-000000000001'::uuid,
      'rogue-agent', 'blank'
    )$$,
  NULL,
  NULL,
  'AC-12: INSERT into foreign org is blocked by RLS'
);

RESET ROLE;

-- ============================================================
-- AC-13: current_tenant() returns correct row for member-a
-- Expected: (Org A, team-x, 'group:member')
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000001","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT row(org_id, group_id, role)::text FROM public.current_tenant() LIMIT 1),
  row(
    '00000000-0000-0000-0000-000000000001'::uuid,
    '00000000-0000-0000-0001-000000000002'::uuid,
    'group:member'
  )::text,
  'AC-13: current_tenant() returns (org_A, team-x, group:member) for member-a'
);

RESET ROLE;

-- ============================================================
-- AC-14: current_tenant() returns 0 rows for user without membership
-- ============================================================
SELECT set_config('request.jwt.claims', '{"sub":"aaaaaaaa-0000-0000-0000-000000000004","role":"authenticated"}', true);
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int FROM public.current_tenant()),
  0,
  'AC-14: current_tenant() returns 0 rows for user without membership'
);

RESET ROLE;

SELECT finish();
ROLLBACK;

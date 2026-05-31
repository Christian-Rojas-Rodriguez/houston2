-- tests/testdata/seed.sql — Task 0010: seed-fixture
--
-- Companion SQL that produces the same MVP fixture state as scripts/seed.go,
-- using service_role context (no RLS). Suitable for direct execution in the
-- Supabase SQL editor or via `supabase db execute`.
--
-- NOTE: auth.users rows cannot be inserted directly via SQL in cloud Supabase
-- (requires the Auth Admin API). The INSERT INTO auth.users block below is
-- provided as documentation / reference for local dev environments where
-- auth.users is accessible.
--
-- All UUIDs are deterministic constants matching tests/helpers/fixture.go.
--
-- Idempotent: uses INSERT ... ON CONFLICT DO NOTHING throughout.
-- Run twice — same state, no errors.

-- ---------------------------------------------------------------------------
-- auth.users (local dev / Supabase local only — not executable on cloud)
-- ---------------------------------------------------------------------------
-- INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at, role)
-- VALUES
--   ('00000010-0000-0000-0001-000000000001', 'user-a@houston-fixture.test', '<bcrypt>', now(), 'authenticated'),
--   ('00000010-0000-0000-0001-000000000002', 'user-b@houston-fixture.test', '<bcrypt>', now(), 'authenticated'),
--   ('00000010-0000-0000-0001-000000000003', 'user-c@houston-fixture.test', '<bcrypt>', now(), 'authenticated')
-- ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- organizations
-- ---------------------------------------------------------------------------
INSERT INTO public.organizations (id, name)
VALUES
  ('00000010-0000-0000-0000-000000000001', 'Houston Fixture Org 1'),
  ('00000010-0000-0000-0000-000000000002', 'Houston Fixture Org 2')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- groups
-- ---------------------------------------------------------------------------
INSERT INTO public.groups (id, org_id, name, is_general)
VALUES
  ('00000010-0000-0001-0000-000000000001', '00000010-0000-0000-0000-000000000001', 'General', true),
  ('00000010-0000-0001-0000-000000000002', '00000010-0000-0000-0000-000000000001', 'Group 2',  false),
  ('00000010-0000-0002-0000-000000000001', '00000010-0000-0000-0000-000000000002', 'General', true)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- memberships
-- ---------------------------------------------------------------------------
INSERT INTO public.memberships (user_id, org_id, group_id, role)
VALUES
  ('00000010-0000-0000-0001-000000000001', '00000010-0000-0000-0000-000000000001', '00000010-0000-0001-0000-000000000001', 'group:member'),
  ('00000010-0000-0000-0001-000000000002', '00000010-0000-0000-0000-000000000001', '00000010-0000-0001-0000-000000000002', 'group:manager'),
  ('00000010-0000-0000-0001-000000000003', '00000010-0000-0000-0000-000000000002', '00000010-0000-0002-0000-000000000001', 'org:owner')
ON CONFLICT (user_id, org_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- agents
-- ---------------------------------------------------------------------------
INSERT INTO public.agents (id, org_id, group_id, source, name)
VALUES
  ('00000010-0000-0000-0002-000000000001', '00000010-0000-0000-0000-000000000001', '00000010-0000-0001-0000-000000000001', 'blank', 'fixture-agent-1'),
  ('00000010-0000-0000-0002-000000000002', '00000010-0000-0000-0000-000000000002', '00000010-0000-0002-0000-000000000001', 'blank', 'fixture-agent-2')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Storage objects cannot be inserted via SQL.
-- Use scripts/seed.go or the Storage REST API with x-upsert:true to create:
--   houston/00000010-0000-0000-0000-000000000001/00000010-0000-0001-0000-000000000001/agents/00000010-0000-0000-0002-000000000001/CLAUDE.md
--   houston/00000010-0000-0000-0000-000000000002/00000010-0000-0002-0000-000000000001/agents/00000010-0000-0000-0002-000000000002/CLAUDE.md
-- ---------------------------------------------------------------------------

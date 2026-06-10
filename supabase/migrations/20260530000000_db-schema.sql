-- Migration: 20260530000000_db-schema
-- Task 0001 — db-schema
-- Creates the six base tables for the Houston 2.0 multi-tenant model,
-- enables pgcrypto, ENABLE+FORCE RLS on all tenant-scoped tables,
-- and the current_tenant() SECURITY DEFINER helper function.
--
-- RFC §4.2 is the authoritative reference for this schema.

-- 1. Extension pgcrypto (required for pgp_sym_encrypt on org_credentials.anthropic_key)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 2. organizations
CREATE TABLE organizations (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       text NOT NULL,
  created_at timestamptz DEFAULT now()
);
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;

-- 3. groups
CREATE TABLE groups (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  name       text NOT NULL,
  is_general boolean NOT NULL DEFAULT false,
  created_at timestamptz DEFAULT now(),
  UNIQUE (org_id, name)
);
ALTER TABLE groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE groups FORCE ROW LEVEL SECURITY;

-- 4. memberships (FK to auth.users — Supabase Auth schema)
CREATE TABLE memberships (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id  uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  org_id   uuid NOT NULL REFERENCES organizations(id),
  group_id uuid NOT NULL REFERENCES groups(id),
  role     text NOT NULL CHECK (role IN ('org:owner', 'group:manager', 'group:member')),
  UNIQUE (user_id, org_id, group_id)
);
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;

-- 5. agents
CREATE TABLE agents (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  group_id   uuid NOT NULL REFERENCES groups(id),
  name       text NOT NULL,
  source     text NOT NULL CHECK (source IN ('blank', 'template', 'ai-assist', 'github')),
  config     jsonb,
  created_at timestamptz DEFAULT now()
);
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents FORCE ROW LEVEL SECURITY;

-- 6. runs
CREATE TABLE runs (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id   uuid NOT NULL REFERENCES agents(id),
  org_id     uuid NOT NULL REFERENCES organizations(id),
  group_id   uuid NOT NULL REFERENCES groups(id),
  user_id    uuid NOT NULL REFERENCES auth.users(id),
  prompt     text NOT NULL,
  result     text,
  status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'error')),
  created_at timestamptz DEFAULT now()
);
ALTER TABLE runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE runs FORCE ROW LEVEL SECURITY;

-- 7. org_credentials
-- anthropic_key is bytea: pgp_sym_encrypt returns bytea; storing as text adds
-- unnecessary base64 round-trip and breaks pgp_sym_decrypt compatibility. (RFC §4.8)
CREATE TABLE org_credentials (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id         uuid NOT NULL REFERENCES organizations(id) UNIQUE,
  anthropic_key  bytea NOT NULL,   -- pgp_sym_encrypt(plaintext, vault_key)
  created_at     timestamptz DEFAULT now(),
  updated_at     timestamptz DEFAULT now()
);
ALTER TABLE org_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_credentials FORCE ROW LEVEL SECURITY;

-- 8. current_tenant() helper function
-- SECURITY DEFINER: runs as owner; allows RLS policies to call it even when
--   the caller lacks direct SELECT on memberships.
-- STABLE: no side effects; result is constant within a transaction — Postgres
--   planner can cache the result per query (important for multi-table RLS).
-- Returns the first membership row for auth.uid(). Task 0002 will layer
-- policies on top of this function.
CREATE OR REPLACE FUNCTION current_tenant()
RETURNS TABLE(org_id uuid, group_id uuid, role text)
LANGUAGE sql
SECURITY DEFINER
STABLE
AS $$
  SELECT m.org_id, m.group_id, m.role
  FROM memberships m
  WHERE m.user_id = auth.uid()
  LIMIT 1;
$$;

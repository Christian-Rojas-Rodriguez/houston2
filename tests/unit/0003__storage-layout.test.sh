#!/usr/bin/env bash
# =============================================================================
# Task 0003 — storage-layout
# Failing-first test suite (QA phase). Must be RED before the migration and
# seed script are applied; GREEN after.
#
# Run:  bash tests/unit/0003__storage-layout.test.sh
# Env:  Local Colima stack must be up (supabase start / colima start)
#
# Acceptance criteria covered:
#   AC-1  bucket houston exists, public=false
#   AC-2  policies houston_tenant_isolation + templates_readonly exist
#   AC-3  group:member of org1 sees 0 objects under houston/<org2>/
#   AC-4  group:member of org1/group1 sees own group + general, NOT group2
#   AC-5  org:owner of org1 sees group1 AND group2, 0 from org2
#   AC-6  templates/sales/CLAUDE.md visible to any authenticated
#   AC-7  INSERT under templates/ from authenticated is rejected
#   AC-8  INSERT with path houston/<org2>/... from org1 session is rejected
#   AC-9  seed uploads template (verified via psql after seed)
#   AC-10 seed is idempotent (runs twice, exit 0 both)
#   AC-11 malformed short path houston/<org1> is NOT visible to authenticated
#   AC-12 current_tenant() exists in pg_proc (precondition)
# =============================================================================

set -uo pipefail

export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"

DB_CONTAINER="supabase_db_houston2"
PSQL_CMD="docker exec -i $DB_CONTAINER psql -U postgres -d postgres"
STORAGE_URL="http://127.0.0.1:54321"

FAIL=0

# -----------------------------------------------------------------------------
# Helper: run psql, capture output, detect TAP failures and ERRORs
# -----------------------------------------------------------------------------
run_pgtap_block() {
  local label="$1"
  local sql="$2"
  local out
  # shellcheck disable=SC2059
  out=$(echo "$sql" | $PSQL_CMD --no-align --tuples-only -f - 2>&1) || true

  echo ""
  echo "=== $label ==="
  echo "$out"

  # Detect any not ok lines (TAP) — psql may emit leading spaces.
  # BSD/macOS grep has no -P; use -E with a POSIX class.
  if echo "$out" | grep -qE '^[[:space:]]*not ok'; then
    echo "FAILED: $label — TAP not-ok detected"
    FAIL=1
  fi
  # Detect psql/Postgres ERROR lines. psql prefixes them as
  # "psql:<stdin>:N: ERROR: ..." so anchor on the ERROR: token, not line start.
  # (throws_ok catches its expected exceptions internally — no ERROR: line emitted.)
  if echo "$out" | grep -q 'ERROR:'; then
    echo "FAILED: $label — ERROR line detected"
    FAIL=1
  fi
}

# =============================================================================
# pgTAP block — all fixture + RLS assertions in one transaction
# =============================================================================

PGTAP_SQL=$(cat <<'EOSQL'
-- Self-contained: install pgtap if needed
CREATE EXTENSION IF NOT EXISTS pgtap;

BEGIN;

-- -------------------------------------------------------------------------
-- AC-12 precondition: current_tenant() must exist BEFORE we touch policies
-- -------------------------------------------------------------------------
SELECT plan(1);
SELECT ok(
  (SELECT count(*) = 1 FROM pg_proc
   WHERE proname = 'current_tenant'
   AND pronamespace = 'public'::regnamespace),
  'AC-12: current_tenant() exists in pg_proc'
);
SELECT finish();

ROLLBACK;
EOSQL
)
run_pgtap_block "AC-12 precondition" "$PGTAP_SQL"

# =============================================================================
# Main pgTAP block — fixture seed + RLS assertions
# =============================================================================
# NOTE: The storage.objects INSERT statements reference bucket_id='houston'.
# That bucket is created by the migration (Task 0003).  Before the migration
# runs, the INSERT fails with a FK violation → the entire pgTAP block exits
# with ERROR → FAIL=1.  This is the intended RED state pre-migration.
#
# Guard approach: we detect whether the bucket exists first.  If it does NOT
# exist we emit a single planned TAP test that immediately fails with a clear
# message, so the test output is clean TAP (not a raw ERROR).  Either way the
# suite exits non-zero.
# =============================================================================

PGTAP_MAIN=$(cat <<'EOSQL'
CREATE EXTENSION IF NOT EXISTS pgtap;

BEGIN;

-- ---- fixture UUIDs (fixed so assertions can reference them) ---------------
DO $$
DECLARE
  _org1   uuid := '00000003-0001-0000-0000-000000000000';
  _org2   uuid := '00000003-0002-0000-0000-000000000000';
  _g1     uuid := '00000003-0011-0000-0000-000000000000';
  _g2     uuid := '00000003-0012-0000-0000-000000000000';
  _g_gen1 uuid := '00000003-0013-0000-0000-000000000000';  -- org1 general
  _gb     uuid := '00000003-0021-0000-0000-000000000000';  -- org2 group
  _u_mem  uuid := '00000003-1001-0000-0000-000000000000';  -- org1/group1 group:member
  _u_own  uuid := '00000003-1002-0000-0000-000000000000';  -- org1 org:owner
  _u_mgr  uuid := '00000003-1003-0000-0000-000000000000';  -- org1/group1 group:manager
  _u_org2 uuid := '00000003-1004-0000-0000-000000000000';  -- org2 member
BEGIN
  -- orgs
  INSERT INTO organizations (id, name) VALUES
    (_org1, 'org-one'),
    (_org2, 'org-two')
  ON CONFLICT (id) DO NOTHING;

  -- groups
  INSERT INTO groups (id, org_id, name, is_general) VALUES
    (_g1,     _org1, 'group-1',  false),
    (_g2,     _org1, 'group-2',  false),
    (_g_gen1, _org1, 'general',  true),
    (_gb,     _org2, 'group-b',  false)
  ON CONFLICT (id) DO NOTHING;

  -- auth.users (minimal: only id required by FK)
  INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at, created_at, updated_at, aud, role)
  VALUES
    (_u_mem,  'mem@test.local',  'x', now(), now(), now(), 'authenticated', 'authenticated'),
    (_u_own,  'own@test.local',  'x', now(), now(), now(), 'authenticated', 'authenticated'),
    (_u_mgr,  'mgr@test.local',  'x', now(), now(), now(), 'authenticated', 'authenticated'),
    (_u_org2, 'org2@test.local', 'x', now(), now(), now(), 'authenticated', 'authenticated')
  ON CONFLICT (id) DO NOTHING;

  -- memberships
  INSERT INTO memberships (id, user_id, org_id, group_id, role) VALUES
    (gen_random_uuid(), _u_mem,  _org1, _g1,    'group:member'),
    (gen_random_uuid(), _u_own,  _org1, _g1,    'org:owner'),
    (gen_random_uuid(), _u_mgr,  _org1, _g1,    'group:manager'),
    (gen_random_uuid(), _u_org2, _org2, _gb,    'group:member')
  ON CONFLICT (user_id, org_id, group_id) DO NOTHING;
END;
$$;

-- ---- Guard: bucket houston must exist (FK for storage.objects inserts) ----
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM storage.buckets WHERE id = 'houston') THEN
    RAISE EXCEPTION 'GUARD: bucket houston does not exist — migration not yet applied';
  END IF;
END;
$$;

-- ---- Fixture objects (inserted as superuser, bypasses RLS) ----------------
DO $$
DECLARE
  _org1   uuid := '00000003-0001-0000-0000-000000000000';
  _org2   uuid := '00000003-0002-0000-0000-000000000000';
  _g1     uuid := '00000003-0011-0000-0000-000000000000';
  _g2     uuid := '00000003-0012-0000-0000-000000000000';
  _gb     uuid := '00000003-0021-0000-0000-000000000000';
BEGIN
  -- org1/group1 agent
  INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
  VALUES (
    '00000003-f001-0000-0000-000000000000',
    'houston',
    'houston/' || _org1::text || '/' || _g1::text || '/agents/a1/CLAUDE.md',
    null, now(), now(), now(), '{}'::jsonb
  ) ON CONFLICT (id) DO NOTHING;

  -- org1/group2 agent (different group, same org)
  INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
  VALUES (
    '00000003-f002-0000-0000-000000000000',
    'houston',
    'houston/' || _org1::text || '/' || _g2::text || '/agents/a2/CLAUDE.md',
    null, now(), now(), now(), '{}'::jsonb
  ) ON CONFLICT (id) DO NOTHING;

  -- org1/general
  INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
  VALUES (
    '00000003-f003-0000-0000-000000000000',
    'houston',
    'houston/' || _org1::text || '/general/notes.md',
    null, now(), now(), now(), '{}'::jsonb
  ) ON CONFLICT (id) DO NOTHING;

  -- org2/group-b agent (other org)
  INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
  VALUES (
    '00000003-f004-0000-0000-000000000000',
    'houston',
    'houston/' || _org2::text || '/' || _gb::text || '/agents/a3/CLAUDE.md',
    null, now(), now(), now(), '{}'::jsonb
  ) ON CONFLICT (id) DO NOTHING;

  -- templates catalog
  INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
  VALUES (
    '00000003-f005-0000-0000-000000000000',
    'houston',
    'templates/sales/CLAUDE.md',
    null, now(), now(), now(), '{}'::jsonb
  ) ON CONFLICT (id) DO NOTHING;

  -- malformed short path (AC-11): only 2 segments after bucket — no group segment
  INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
  VALUES (
    '00000003-f006-0000-0000-000000000000',
    'houston',
    'houston/' || _org1::text,
    null, now(), now(), now(), '{}'::jsonb
  ) ON CONFLICT (id) DO NOTHING;
END;
$$;

-- ---- Plan ------------------------------------------------------------------
-- AC-1(2) + AC-2(1) + AC-3(1) + AC-4(3) + AC-5(3) + AC-6(1) + AC-7(1) + AC-8(1) + AC-11(1) = 14
SELECT plan(14);

-- ===========================================================================
-- AC-1: bucket houston exists and public=false
-- ===========================================================================
SELECT ok(
  (SELECT count(*) = 1 FROM storage.buckets WHERE id = 'houston'),
  'AC-1a: bucket houston exists'
);
SELECT ok(
  (SELECT public = false FROM storage.buckets WHERE id = 'houston'),
  'AC-1b: bucket houston is not public'
);

-- ===========================================================================
-- AC-2: both policies exist on storage.objects
-- ===========================================================================
SELECT ok(
  (SELECT count(*) = 2
   FROM pg_policies
   WHERE schemaname = 'storage'
     AND tablename = 'objects'
     AND policyname IN ('houston_tenant_isolation', 'templates_readonly')),
  'AC-2: houston_tenant_isolation and templates_readonly policies exist'
);

-- ===========================================================================
-- AC-3: group:member of org1 sees 0 objects under houston/<org2>/
-- ===========================================================================
DO $$
DECLARE
  _u_mem  uuid := '00000003-1001-0000-0000-000000000000';
  _org2   uuid := '00000003-0002-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_mem::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/' || '00000003-0002-0000-0000-000000000000' || '/%'),
  0,
  'AC-3: org1/group1 member sees 0 objects under houston/org2/'
);

RESET ROLE;

-- ===========================================================================
-- AC-4: group:member of org1/group1 sees own group + general, NOT group2
-- ===========================================================================
DO $$
DECLARE
  _u_mem uuid := '00000003-1001-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_mem::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

-- must see own group
SELECT ok(
  (SELECT count(*) > 0
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/00000003-0001-0000-0000-000000000000/00000003-0011-0000-0000-000000000000/%'),
  'AC-4a: group:member sees objects in own group (group1)'
);

-- must see general
SELECT ok(
  (SELECT count(*) > 0
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/00000003-0001-0000-0000-000000000000/general/%'),
  'AC-4b: group:member sees objects in org1/general'
);

-- must NOT see group2
SELECT is(
  (SELECT count(*)::int
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/00000003-0001-0000-0000-000000000000/00000003-0012-0000-0000-000000000000/%'),
  0,
  'AC-4c: group:member sees 0 objects in org1/group2 (foreign group)'
);

RESET ROLE;

-- ===========================================================================
-- AC-5: org:owner of org1 sees group1 AND group2, 0 from org2
-- ===========================================================================
DO $$
DECLARE
  _u_own uuid := '00000003-1002-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_own::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

SELECT ok(
  (SELECT count(*) > 0
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/00000003-0001-0000-0000-000000000000/00000003-0011-0000-0000-000000000000/%'),
  'AC-5a: org:owner sees objects in org1/group1'
);

SELECT ok(
  (SELECT count(*) > 0
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/00000003-0001-0000-0000-000000000000/00000003-0012-0000-0000-000000000000/%'),
  'AC-5b: org:owner sees objects in org1/group2'
);

SELECT is(
  (SELECT count(*)::int
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'houston/00000003-0002-0000-0000-000000000000/%'),
  0,
  'AC-5c: org:owner sees 0 objects under org2'
);

RESET ROLE;

-- ===========================================================================
-- AC-6: templates/sales/CLAUDE.md visible to any authenticated user
-- ===========================================================================
DO $$
DECLARE
  _u_mem uuid := '00000003-1001-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_mem::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

SELECT ok(
  (SELECT count(*) >= 1
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name LIKE 'templates/%'),
  'AC-6: templates/sales/CLAUDE.md visible to authenticated user'
);

RESET ROLE;

-- ===========================================================================
-- AC-7: INSERT under templates/ from authenticated is rejected
-- ===========================================================================
DO $$
DECLARE
  _u_mem uuid := '00000003-1001-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_mem::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

SELECT throws_ok(
  $$INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
    VALUES (
      gen_random_uuid(),
      'houston',
      'templates/evil/inject.md',
      null, now(), now(), now(), '{}'::jsonb
    )$$,
  'AC-7: INSERT under templates/ from authenticated is rejected by RLS'
);

RESET ROLE;

-- ===========================================================================
-- AC-8: INSERT cross-tenant (houston/<org2>/...) from org1 session is rejected
-- ===========================================================================
DO $$
DECLARE
  _u_mem uuid := '00000003-1001-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_mem::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

SELECT throws_ok(
  $$INSERT INTO storage.objects (id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata)
    VALUES (
      gen_random_uuid(),
      'houston',
      'houston/00000003-0002-0000-0000-000000000000/00000003-0021-0000-0000-000000000000/agents/hack/CLAUDE.md',
      null, now(), now(), now(), '{}'::jsonb
    )$$,
  'AC-8: cross-tenant INSERT into org2 path from org1 session is rejected'
);

RESET ROLE;

-- ===========================================================================
-- AC-11: malformed short path (houston/<org1>) not visible to authenticated
-- ===========================================================================
DO $$
DECLARE
  _u_mem uuid := '00000003-1001-0000-0000-000000000000';
BEGIN
  PERFORM set_config(
    'request.jwt.claims',
    json_build_object('sub', _u_mem::text, 'role', 'authenticated')::text,
    true
  );
END;
$$;
SET LOCAL ROLE authenticated;

SELECT is(
  (SELECT count(*)::int
   FROM storage.objects
   WHERE bucket_id = 'houston'
     AND name = 'houston/00000003-0001-0000-0000-000000000000'),
  0,
  'AC-11: malformed short path houston/<org1> is not visible to authenticated'
);

RESET ROLE;

SELECT finish();
ROLLBACK;
EOSQL
)

run_pgtap_block "AC-1..AC-11 (RLS assertions)" "$PGTAP_MAIN"

# =============================================================================
# AC-9 / AC-10 — seed script verification
# =============================================================================
echo ""
echo "=== AC-9 / AC-10: seed script verification ==="

SEED_SCRIPT="scripts/seed-templates.sh"

if [ ! -f "$SEED_SCRIPT" ]; then
  echo "FAILED: $SEED_SCRIPT does not exist — coder has not implemented it yet (correct RED state)"
  FAIL=1
else
  # Obtain service_role key dynamically from supabase status
  SUPABASE_SERVICE_ROLE_KEY=""
  if command -v supabase &>/dev/null; then
    SUPABASE_SERVICE_ROLE_KEY=$(supabase status -o env 2>/dev/null | grep SERVICE_ROLE_KEY | cut -d= -f2 | tr -d '"' || true)
  fi

  if [ -z "$SUPABASE_SERVICE_ROLE_KEY" ]; then
    echo "FAILED: could not obtain SERVICE_ROLE_KEY from supabase status"
    FAIL=1
  else
    export SUPABASE_SERVICE_ROLE_KEY
    export SUPABASE_URL="http://127.0.0.1:54321"

    # AC-10a: first run
    if bash "$SEED_SCRIPT"; then
      echo "AC-10a: first seed run exited 0 — OK"
    else
      echo "FAILED: AC-10a — first seed run exited non-zero"
      FAIL=1
    fi

    # AC-10b: second run (idempotence)
    if bash "$SEED_SCRIPT"; then
      echo "AC-10b: second seed run exited 0 (idempotent) — OK"
    else
      echo "FAILED: AC-10b — second seed run exited non-zero (not idempotent)"
      FAIL=1
    fi

    # AC-9: verify template row exists in storage.objects via psql (no JWT needed)
    AC9_SQL="SELECT count(*) FROM storage.objects WHERE bucket_id='houston' AND name='templates/sales/CLAUDE.md';"
    AC9_COUNT=$(echo "$AC9_SQL" | $PSQL_CMD --no-align --tuples-only -f - 2>&1 | tr -d '[:space:]')

    if [ "$AC9_COUNT" = "1" ]; then
      echo "AC-9: storage.objects row exists for templates/sales/CLAUDE.md — OK"
    else
      echo "FAILED: AC-9 — expected 1 row in storage.objects for templates/sales/CLAUDE.md, got '$AC9_COUNT'"
      FAIL=1
    fi
  fi
fi

# =============================================================================
# Final result
# =============================================================================
echo ""
if [ "$FAIL" -ne 0 ]; then
  echo "RESULT: FAIL — one or more assertions failed (see above)"
  exit 1
else
  echo "RESULT: PASS — all assertions passed"
  exit 0
fi

#!/usr/bin/env bash
# scripts/seed-templates.sh
# Task 0003 — storage-layout
#
# Idempotent upload of templates/sales/CLAUDE.md to bucket 'houston'
# via the Supabase Storage REST API using service_role (bypasses RLS).
#
# Usage:
#   export SUPABASE_SERVICE_ROLE_KEY=<key>
#   export SUPABASE_URL=http://127.0.0.1:54321  # optional, defaults to local
#   bash scripts/seed-templates.sh
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

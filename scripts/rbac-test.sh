#!/usr/bin/env bash
# Task 0012 — rbac-test acceptance gate.
#
# Requiere un Supabase vivo + entorno:
#   SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, SUPABASE_ANON_KEY  (y SUPABASE_JWT_SECRET)
# (NO se auto-sourcea .env para no correr el seed contra cloud por accidente.)
#
# Exit 0 = RBAC correcto. Exit 1 = violación de RBAC. Exit 2 = entorno no configurado.
set -uo pipefail
cd "$(dirname "$0")/.."

if [ -z "${SUPABASE_URL:-}" ] || [ -z "${SUPABASE_SERVICE_ROLE_KEY:-}" ] || [ -z "${SUPABASE_ANON_KEY:-}" ]; then
  echo "rbac-test: faltan SUPABASE_URL / SUPABASE_SERVICE_ROLE_KEY / SUPABASE_ANON_KEY — abortando" >&2
  exit 2
fi

go test -tags=acceptance -run TestRBAC ./tests/acceptance/... -v
code=$?

if [ "$code" -eq 0 ]; then
  echo "rbac-test: PASS — RBAC correcto"
else
  echo "rbac-test: FAIL — violación de RBAC (ver salida arriba)"
fi
exit "$code"

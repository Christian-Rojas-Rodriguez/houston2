#!/usr/bin/env bash
# Task 0011 — leak-test acceptance gate.
#
# Requiere un Supabase vivo + entorno:
#   SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, SUPABASE_ANON_KEY
# (NO se auto-sourcea .env para evitar correr el seed contra cloud por accidente;
#  seteá el entorno del Supabase objetivo explícitamente.)
#
# Exit 0 = aislamiento íntegro. Exit 1 = posible leak (la salida del test imprime
# la tabla/objeto ofensor). Exit 2 = entorno no configurado.
set -uo pipefail
cd "$(dirname "$0")/.."

if [ -z "${SUPABASE_URL:-}" ] || [ -z "${SUPABASE_SERVICE_ROLE_KEY:-}" ] || [ -z "${SUPABASE_ANON_KEY:-}" ]; then
  echo "leak-test: faltan SUPABASE_URL / SUPABASE_SERVICE_ROLE_KEY / SUPABASE_ANON_KEY — abortando" >&2
  exit 2
fi

go test -tags=acceptance -run TestLeak ./tests/acceptance/... -v
code=$?

if [ "$code" -eq 0 ]; then
  echo "leak-test: PASS — aislamiento íntegro"
else
  echo "leak-test: FAIL — posible leak (ver salida arriba)"
fi
exit "$code"

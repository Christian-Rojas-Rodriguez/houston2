#!/usr/bin/env bash
# Houston 2.0 — setup de la demo (una sola vez por sesión).
# Levanta Supabase local, aplica migraciones y siembra el fixture.
set -euo pipefail
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
cd "$(dirname "$0")/.."

echo "▶ Colima/Docker..."
command -v colima >/dev/null && colima status >/dev/null 2>&1 || { echo "  arrancá Colima:  colima start --cpu 4 --memory 8"; }

echo "▶ Supabase local (start; -x vector,analytics)..."
supabase start -x vector,analytics 2>&1 | tail -1 || true

echo "▶ Migraciones (db reset)..."
supabase db reset 2>&1 | tail -2

echo "▶ Sembrando fixture (2 orgs / 3 usuarios / 2 agentes / Storage)..."
eval "$(supabase status -o env 2>/dev/null | sed 's/^/export SB_/')"
SUPABASE_URL="http://127.0.0.1:54321" SUPABASE_SERVICE_ROLE_KEY="$SB_SERVICE_ROLE_KEY" \
  go run ./scripts/seed.go | tail -1

echo ""
echo "✅ Setup listo."
echo "   Terminal 1:  bash demo/serve.sh      # levanta el orquestador (dejala corriendo)"
echo "   Terminal 2:  bash demo/run-demo.sh   # corre la demo e2e"

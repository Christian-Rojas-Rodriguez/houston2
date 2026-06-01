#!/usr/bin/env bash
# Houston 2.0 — levanta el orquestador para la demo (dejá esta terminal corriendo).
# Lee la API key de .env y las creds de Supabase local de `supabase status`.
set -uo pipefail
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
cd "$(dirname "$0")/.."

eval "$(supabase status -o env 2>/dev/null | sed 's/^/export SB_/')"
export SUPABASE_URL="http://127.0.0.1:54321"
export SUPABASE_ANON_KEY="${SB_ANON_KEY:-}"
export SUPABASE_JWT_SECRET="${SB_JWT_SECRET:-}"
export ANTHROPIC_API_KEY="$(grep '^ANTHROPIC_API_KEY=' .env 2>/dev/null | cut -d= -f2- | tr -d '"')"
export SERVER_PORT="${SERVER_PORT:-8080}"

if [ -z "$ANTHROPIC_API_KEY" ]; then echo "⚠️  falta ANTHROPIC_API_KEY en .env"; exit 1; fi
if ! command -v claude >/dev/null; then echo "⚠️  no encuentro el binario 'claude' en el PATH"; exit 1; fi

echo "▶ orquestador en http://localhost:${SERVER_PORT}   (claude: $(command -v claude))"
echo "   Ctrl-C para parar. En otra terminal: bash demo/run-demo.sh"
exec go run ./cmd/server

#!/usr/bin/env bash
# Houston 2.0 — runner e2e de la demo (curl; reemplaza Postman).
# Requiere: demo/serve.sh corriendo + Supabase local + fixture sembrado (demo/setup.sh).
set -uo pipefail
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
cd "$(dirname "$0")/.."

SERVER="${SERVER_URL:-http://localhost:8080}"
SB="${SUPABASE_URL:-http://127.0.0.1:54321}"
eval "$(supabase status -o env 2>/dev/null | sed 's/^/export SB_/')" || true
ANON="${SUPABASE_ANON_KEY:-${SB_ANON_KEY:-}}"
SVC="${SB_SERVICE_ROLE_KEY:-}"

AGENT_ORG1="00000010-0000-0000-0002-000000000001"   # agente de A (Org1/grupo general)
AGENT_ORG2="00000010-0000-0000-0002-000000000002"   # agente de otro tenant (Org2)
ORG1="00000010-0000-0000-0000-000000000001"
ORG2="00000010-0000-0000-0000-000000000002"

G='\033[1;32m'; R='\033[1;31m'; C='\033[1;36m'; Y='\033[1;33m'; B='\033[1m'; N='\033[0m'
FAILED=0
pass(){ echo -e "  ${G}✓${N} $1"; }
fail(){ echo -e "  ${R}✗ $1${N}"; FAILED=1; }
hdr(){  echo -e "\n${C}${B}▌ $1${N}"; }

login(){ curl -s -X POST "$SB/auth/v1/token?grant_type=password" -H "apikey: $ANON" -H "Content-Type: application/json" \
  -d "{\"email\":\"$1\",\"password\":\"$2\"}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4; }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('$1',''))" 2>/dev/null; }

echo -e "${B}Houston 2.0 — Demo e2e (tenant + Supabase + Claude)${N}"

hdr "0 · Health"
code=$(curl -s -o /dev/null -w '%{http_code}' "$SERVER/health" || echo 000)
[ "$code" = 200 ] && pass "orquestador vivo (200)" || { fail "server no responde ($code) — ¿corriste 'bash demo/serve.sh'?"; exit 1; }

hdr "1 · Auth — 3 actores reales (JWT ES256 de Supabase)"
MEM=$(login user-a@houston-fixture.test houston-fixture-pass-A)
MGR=$(login user-b@houston-fixture.test houston-fixture-pass-B)
OWN=$(login user-c@houston-fixture.test houston-fixture-pass-C)
[ -n "$MEM" ] && pass "member (A) logueado" || fail "no pude loguear member — ¿corriste 'bash demo/setup.sh'?"
[ -n "$MGR" ] && pass "manager (B) logueado" || fail "manager"
[ -n "$OWN" ] && pass "owner (C) logueado" || fail "owner"

hdr "2 · RBAC — member intenta crear agente"
code=$(curl -s -o /tmp/_d -w '%{http_code}' -X POST "$SERVER/v1/agents" -H "Authorization: Bearer $MEM" \
  -H "Content-Type: application/json" -d '{"name":"intento-member","source":"blank"}')
[ "$code" = 403 ] && pass "403 forbidden — el rol bloquea (server-side)" || fail "esperaba 403, obtuve $code"

hdr "3 · Crear agente (manager)"
resp=$(curl -s -X POST "$SERVER/v1/agents" -H "Authorization: Bearer $MGR" \
  -H "Content-Type: application/json" -d '{"name":"Demo Sales Agent","source":"blank"}')
NEWID=$(echo "$resp" | jget id)
[ -n "$NEWID" ] && pass "201 — agente creado: ${NEWID:0:8}…" || fail "no se creó: $resp"

hdr "4 · ⭐ Correr agente — Claude Code REAL"
echo -e "  ${Y}(tarda ~5–10s: es una sesión real de Claude Code)${N}"
resp=$(curl -s -X POST "$SERVER/v1/agents/$AGENT_ORG1/runs" -H "Authorization: Bearer $MEM" \
  -H "Content-Type: application/json" -d '{"prompt":"Leé tu CLAUDE.md de contexto. ¿Para qué org y grupo estás configurado? Respondé en una sola línea."}')
status=$(echo "$resp" | jget status); result=$(echo "$resp" | jget result)
if [ "$status" = "done" ]; then pass "run completado (status=done)"; echo -e "  ${B}🤖 CLAUDE:${N} ${result}"; else fail "el run no completó: $resp"; fi

hdr "5 · Aislamiento — member intenta correr agente de OTRO tenant"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$SERVER/v1/agents/$AGENT_ORG2/runs" -H "Authorization: Bearer $MEM" \
  -H "Content-Type: application/json" -d '{"prompt":"no debería"}')
[ "$code" = 404 ] && pass "404 — A ni siquiera VE el agente de Org2 (RLS)" || fail "esperaba 404, obtuve $code"

hdr "5b · Aislamiento a nivel datos — member ve solo su org"
orgs=$(curl -s "$SB/rest/v1/organizations?select=id" -H "apikey: $ANON" -H "Authorization: Bearer $MEM")
echo "$orgs" | grep -q "$ORG1" && pass "ve su propia org (Org1)" || fail "no ve su org"
if echo "$orgs" | grep -q "$ORG2"; then fail "FUGA: ve Org2!"; else pass "NO ve Org2 — cero fuga, garantizado por RLS"; fi

# cleanup del agente creado en el paso 3 (para re-correr la demo limpio)
if [ -n "$NEWID" ] && [ -n "$SVC" ]; then
  curl -s -o /dev/null -X DELETE "$SB/rest/v1/agents?id=eq.$NEWID" -H "apikey: $SVC" -H "Authorization: Bearer $SVC" -H "Prefer: return=minimal"
fi

echo ""
if [ "$FAILED" = 0 ]; then echo -e "${G}${B}✓ DEMO OK — todo verde. Crear → Correr (Claude real) → Aislado.${N}"; else echo -e "${R}${B}✗ Algo falló (ver arriba).${N}"; exit 1; fi

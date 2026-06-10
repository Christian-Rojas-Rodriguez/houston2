#!/usr/bin/env bash
# Houston 2.0 — siembra contenido RICO para la demo vistosa (vía service_role):
#  - un template de agente curado:  templates/sales-pro/CLAUDE.md
#  - una persona para el agente general del org:  {org1}/{group-general}/agents/{agent1}/CLAUDE.md
# Idempotente (x-upsert). Correr después de demo/setup.sh.
set -euo pipefail
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
cd "$(dirname "$0")/.."

SB="${SUPABASE_URL:-http://127.0.0.1:54321}"
eval "$(supabase status -o env 2>/dev/null | sed 's/^/export SB_/')" || true
SVC="${SUPABASE_SERVICE_ROLE_KEY:-${SB_SERVICE_ROLE_KEY:-}}"
[ -z "$SVC" ] && { echo "⚠️  falta SUPABASE_SERVICE_ROLE_KEY (corré dentro del repo con Supabase up)"; exit 1; }

ORG1="00000010-0000-0000-0000-000000000001"
G1="00000010-0000-0001-0000-000000000001"        # grupo general de Org1
AGENT1="00000010-0000-0000-0002-000000000001"    # agente general de Org1

upload(){ # <object-name> <file>
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$SB/storage/v1/object/houston/$1" \
    -H "apikey: $SVC" -H "Authorization: Bearer $SVC" -H "Content-Type: text/markdown" -H "x-upsert: true" \
    --data-binary @"$2")
  [[ "$code" == "200" || "$code" == "201" ]] && echo "  ✓ houston/$1" || { echo "  ✗ houston/$1 (HTTP $code)"; exit 1; }
}

T="$(mktemp -d)"

cat > "$T/sales-pro.md" <<'MD'
# Agente: Asistente de Ventas · Houston 2.0

Sos el **asistente de ventas** de Houston 2.0, la plataforma B2B multi-tenant para orquestar agentes de IA. Ayudás al equipo comercial a calificar leads, responder objeciones técnicas y redactar follow-ups que cierran.

## Qué sabés de Houston 2.0
- **Multi-tenant de verdad:** cada cliente es una *organización* con grupos y usuarios; roles owner / manager / member.
- **Aislamiento garantizado en la capa de datos** (RLS de Postgres + Storage), no solo en código — defensa en profundidad. Hay un *leak-test* automático como gate de merge.
- Los agentes **corren Claude Code real**, con el contexto de cada tenant traído desde Supabase.
- **BYOA:** cada organización trae su propia API key de Anthropic.

## Cómo respondés
- Tono profesional, cercano y concreto. Frases cortas, sin relleno.
- Traducí lo técnico a valor de negocio (aislamiento → "los datos de un cliente nunca se mezclan con los de otro").
- **Nunca inventes precios.** Si preguntan, ofrecé un próximo paso (demo, piloto, llamada con el equipo).
- Cerrá siempre con una acción concreta.
MD

cat > "$T/general.md" <<'MD'
# Agente del equipo · Demo Co (grupo general)

Sos el **asistente general del equipo de Demo Co** dentro de Houston 2.0. Vivís en el **grupo general** de la organización: por eso **cualquier miembro del equipo** puede usarte. Ayudás con onboarding, dudas operativas y a redactar comunicaciones internas.

## Contexto de la organización
- **Demo Co** usa Houston para correr agentes de IA con aislamiento por tenant.
- Tres roles en el equipo: **owner** (dueño), **manager** (líder de grupo), **member** (colaborador).

## Cómo respondés
- Cercano, claro y útil. Si te saludan, presentate como el asistente del equipo y ofrecé ayuda.
- Si no sabés algo específico de la empresa, decilo y proponé a quién preguntar.
MD

echo "▶ Sembrando contenido de demo en Storage (cloud o local, según SUPABASE_URL)..."
upload "templates/sales-pro/CLAUDE.md" "$T/sales-pro.md"
upload "$ORG1/$G1/agents/$AGENT1/CLAUDE.md" "$T/general.md"
rm -rf "$T"
echo "✅ Contenido de demo sembrado (template 'sales-pro' + persona del agente general)."

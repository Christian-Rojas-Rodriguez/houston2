# Houston 2.0 — Runbook de la demo e2e

Guía para correr una demo **exitosa** del control plane multi-tenant (tenant + Supabase + Claude Code) frente al founder/CTO de Houston, usando la colección Postman `houston2-e2e.postman_collection.json`.

---

## 0. Qué vas a mostrar (el arco narrativo)

> "Convertimos Houston —single-tenant, local— en una plataforma **B2B multi-tenant**, con aislamiento garantizado en la **capa de datos** (no solo en código), y agentes que corren **Claude Code real** con el contexto de cada tenant."

5 momentos, en orden:
1. **Auth** — 3 usuarios reales (member / manager / owner) se loguean (tokens ES256 de Supabase).
2. **RBAC** — un `member` intenta crear un agente → **403** (el rol manda).
3. **Crear** — un `manager` crea un agente (fila en DB + `CLAUDE.md` en Storage).
4. **⭐ Correr** — un usuario corre el agente y **Claude Code levanta de verdad**, leyendo el contexto desde Supabase Storage.
5. **Aislamiento** — el mismo usuario intenta tocar otro tenant → **404 / 0 filas**. La fuga es imposible.

---

## 1. Pre-requisitos (una sola vez)

- **`claude` CLI** instalado y funcionando (`claude --print "hola"` debe responder). Es lo que el orquestador lanza como subprocess.
- **Go** (1.22+) para correr el orquestador.
- **Supabase** — una de dos:
  - **Local (recomendado para la demo: rápido, sin red):** Colima + `supabase start` (con `-x vector,analytics`).
  - **Cloud (más impactante: tu proyecto real):** ya migrado y con el fixture sembrado.
- **API key de Anthropic** en `.env` como `ANTHROPIC_API_KEY=sk-ant-...`.

---

## 2. Setup (copy-paste)

### Opción A — Local (Colima)

```bash
cd <repo>
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"   # para que el server encuentre `claude`

# 1) Supabase local arriba + migraciones limpias
supabase start -x vector,analytics
supabase db reset            # aplica las 5 migraciones (schema, RLS, storage, + fixes)

# 2) Sembrar el fixture (2 orgs / 3 usuarios / 2 agentes / Storage)
eval "$(supabase status -o env | sed 's/^/export SB_/')"
SUPABASE_URL=http://127.0.0.1:54321 SUPABASE_SERVICE_ROLE_KEY="$SB_SERVICE_ROLE_KEY" \
  go run ./scripts/seed.go

# 3) Levantar el orquestador (deja esta terminal corriendo)
SUPABASE_URL=http://127.0.0.1:54321 \
SUPABASE_ANON_KEY="$SB_ANON_KEY" \
SUPABASE_JWT_SECRET="$SB_JWT_SECRET" \
ANTHROPIC_API_KEY="$(grep '^ANTHROPIC_API_KEY=' .env | cut -d= -f2- | tr -d '\"')" \
SERVER_PORT=8080 \
  go run ./cmd/server
```

Variables Postman para esta opción:
- `server_url` = `http://localhost:8080`
- `supabase_url` = `http://127.0.0.1:54321`
- `supabase_anon_key` = el valor de `SB_ANON_KEY` (de `supabase status -o env`)

### Opción B — Cloud (tu proyecto real)

```bash
cd <repo>
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"
set -a; source .env; set +a
export SUPABASE_URL="${SUPABASE_URL%/}"      # IMPORTANTE: sin / final

# Sembrar el fixture en cloud (una vez; idempotente)
go run ./scripts/seed.go

# Levantar el orquestador apuntando a cloud
SERVER_PORT=8080 go run ./cmd/server
```

Variables Postman: `server_url` = `http://localhost:8080`, `supabase_url` = tu URL cloud (sin `/` final), `supabase_anon_key` = tu anon key.

> **Tip de confiabilidad:** ensayá la Opción A (local) **al menos una vez antes** de la demo. Si la red/cloud falla en vivo, local te salva.

---

## 3. Correr la demo (Postman)

1. Importá `demo/houston2-e2e.postman_collection.json`.
2. Seteá las 3 variables (`server_url`, `supabase_url`, `supabase_anon_key`).
3. Andá carpeta por carpeta, **en orden**, ejecutando cada request (o usá **Run Collection** para verlo todo verde de una). Cada request tiene asserts (los ✓ verdes son parte del show).

### Guión (qué decir en cada paso)

| Carpeta | Acción | Qué decir |
|---|---|---|
| **0 Health** | `GET /health` | "El orquestador Go está vivo. Es la capa que mete identidad, RBAC y orquesta a Claude." |
| **1 Auth** | Login Member/Manager/Owner | "Tres usuarios reales, tres roles. El token es un JWT firmado por Supabase (ES256). En prod esto es Google SSO; acá uso usuarios de fixture." |
| **2 RBAC** | Member crea agente → **403** | "El control de acceso no es decorativo: un `member` no puede crear agentes. 403, y el handler nunca corre. El rol se deriva de la membership, server-side." |
| **3 Crear** | Manager crea agente → **201** | "Un `manager` sí puede. El agente nace scoped a su org+grupo: una fila en Postgres y su `CLAUDE.md` en Storage, ambos protegidos por RLS." |
| **4 ⭐ Correr** | Member corre el agente → **200** | "Y acá está la magia: el orquestador baja el contexto del agente **desde Supabase Storage**, levanta **Claude Code de verdad** como subprocess con la API key inyectada, y captura la respuesta. Miren el `result`: Claude leyó el contexto del tenant y respondió." *(abrir la consola de Postman para ver el `🤖 CLAUDE:` log)* |
| **5 Aislamiento** | Cross-org run → **404**; data-level → solo su org | "Lo más importante para un SaaS B2B: el aislamiento. A intenta correr un agente de otro tenant → 404, ni lo ve. Y a nivel de datos, A solo ve su org; la otra org **nunca** aparece. Esto está garantizado por **RLS de Postgres**, no por un `if` en el código. Tenemos un *leak-test* automático que es gate de merge." |

---

## 4. Fallbacks (si algo se complica en vivo)

- **El run (paso 4) tarda** (~5–10s): es Claude Code corriendo de verdad. Avisá: "está levantando una sesión real de Claude". Si querés, bajá la expectativa del prompt (algo cortito).
- **El run falla con 502 / "ANTHROPIC_API_KEY not configured":** el server no tiene la key. Verificá que arrancó con `ANTHROPIC_API_KEY` en el env.
- **401 en los `/v1/*`:** el token no validó. Asegurate de que el server tenga `SUPABASE_URL` correcto (valida el ES256 contra el JWKS de ese Supabase) y que `supabase_url` en Postman apunte al **mismo** Supabase del que sacás el token.
- **404 inesperado en el run "permitido":** el fixture no está sembrado (corré `go run ./scripts/seed.go`).
- **Plan B sin Postman:** los mismos pasos están como tests automáticos verdes — `bash scripts/leak-test.sh` (aislamiento) y `bash scripts/rbac-test.sh` (RBAC). Mostrar la suite en verde también vende.

---

## 5. Cierre (el remate)

> "Todo esto —identidad, RBAC, aislamiento por datos, agentes corriendo Claude Code con contexto por tenant— está construido con **spec-as-source** y **TDD**, y verificado end-to-end contra Supabase real. De hecho, correrlo de verdad nos hizo cazar 4 bugs que los tests mockeados no veían. Houston pasa de single-tenant a una plataforma B2B multi-tenant, sin perder lo que ya funciona."

Pasá a la presentación (`demo/presentation.html`) para el contexto de arquitectura.

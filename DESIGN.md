# Houston 2.0 — Design (HOW)

> `README.md` describes **what** we are building and **why**. This document describes **how**:
> the runtime model, the hosting/sandbox candidates, and how Supabase enforces isolation.
> It is a decision-support document — the final runtime and hosting picks are still open
> (see §9).

---

## 1. Purpose & relationship to the README

The `README.md` defines Houston 2.0's *what/why*: a multi-tenant platform hosting 1..N AI
agents for N organizations, with hard isolation by `org + group`, a **stateless engine**, and
a knowledge base served by semantic search. It deliberately leaves the *how* open.

This document fills that gap, and in doing so confronts a tension the README glosses over:

> The README states **"an agent is a configuration, not a process"** and **"the engine is
> stateless"**. But the agent runtime we actually want to run — Claude Code / the Claude
> Agent SDK — is a **stateful process**: it has a working directory (`files, images,
> scripts`) and a `~/.claude` home (memory, skills, commands, session history). The
> whiteboard sketch names this pain directly: *how do we manage Claude's local files /
> memory? how do we isolate `~/.claude`? how do we recover everything from Supabase?*

**Resolution adopted here:** keep the **control plane stateless** (Go), and make each
**agent runtime an ephemeral, hydrated-on-demand instance**. All durable state lives in
Supabase; the instance itself is disposable. This preserves the README's "stateless engine"
principle — it just relocates statefulness out of the long-lived process and into Supabase.

In one line:

```
Houston 1:  n organizations = n VPS   (dedicated, always-on, manual, idle waste)
Houston 2:  1 control plane + N ephemeral, isolated, hydrated micro-instances
            (managed at the infrastructure level, scale-to-zero)
```

---

## 2. Topology

```
Client
  │  request carrying a verifiable identity
  ▼
┌──────────────────────────────────────────────────────────────────────┐
│  Go control plane  (stateless)                                         │
│  • auth: verify identity, derive tenant context (org + group)          │
│  • mint a SHORT-LIVED token scoped to exactly one org/group            │
│  • provision / route to an ephemeral sandbox instance                  │
│  • meter usage                                                         │
└───────────────┬────────────────────────────────────────────────────────┘
                │ provisions
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│  Ephemeral sandbox instance  (disposable)                              │
│  1. hydrate workspace + ~/.claude from Supabase (scoped to org/group)  │
│  2. run the agent  (runtime = open decision, see §3)                   │
│  3. sync outputs / session / memory / usage back to Supabase           │
│  4. tear down (scale to zero when idle)                                │
└───────────────┬────────────────────────────────────────────────────────┘
                │ read (scoped token) / write
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│  Supabase  =  the hard isolation boundary                              │
│  Postgres + RLS   ·   pgvector   ·   Storage buckets   ·   Auth        │
└──────────────────────────────────────────────────────────────────────┘
```

**Mapping back to README principles:**

| README principle | How this topology satisfies it |
|---|---|
| "The engine is stateless." | The Go control plane holds no state; every instance is hydrated from Supabase and torn down. |
| "Isolation is enforced by the data layer." | Postgres RLS + per-instance scoped token are the hard guarantee; the control plane is the first barrier. |
| "Compute cost only appears when an agent receives a request." | Ephemeral instances scale to zero; we pay only while an agent is actively working. |
| "An agent is a configuration, not a process." | The *definition* (instructions, skills, knowledge scope) lives in Supabase; the process is a transient detail hydrated from that definition. |

---

## 3. Runtime — three options to research (NOT yet decided)

How the agent loop actually runs. These are presented as candidates to evaluate, not a
settled choice.

### Option A — Claude Agent SDK (headless)

The same agentic loop and tools that power Claude Code, run non-interactively (`-p` /
`query()` async generator). Claude-native memory, skills, subagents, extended thinking, and
prompt caching are wired in — we don't rebuild them.

- **Key cross-host fact:** to move a session across hosts, persist
  `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` to durable storage and restore it
  (to the same `cwd`) before calling `resume`. This is the concrete answer to *"isolate
  `~/.claude`"* and *"recover everything from Supabase"*: `~/.claude` becomes a **hydrated
  artifact stored in Supabase**, not a long-lived disk.
- **Best when:** we want Claude-native agent behavior with the least custom plumbing.
- **Trade-off:** ties the runtime to Claude; the session/memory persistence contract is ours
  to operate.
- **Auth note:** Option A can authenticate via the user's Anthropic API key (§6) set as
  `ANTHROPIC_API_KEY` environment variable on the instance. The Agent SDK accepts this
  alongside its OAuth path.
- **Resources:**
  - https://code.claude.com/docs/en/headless
  - https://platform.claude.com/docs/en/agent-sdk/sessions
  - https://platform.claude.com/docs/en/agent-sdk/hosting

### Option B — Vercel AI SDK (Agent abstraction)

A TypeScript toolkit for AI-powered apps. Its `Agent` abstraction defines reusable agents
with tools, instructions, and type-safe streaming, and it can use **Claude as a provider**.
Strong fit if a web product surface (chat UI, streaming dashboards) is primary.

- **Best when:** the product is web-first and we want first-class frontend streaming.
- **Trade-off:** lighter agent layer — we own more of the loop, tool wiring, and any
  Claude-native memory/skills we want.
- **Complementary note:** B and Claude are not mutually exclusive — the Vercel AI SDK can run
  the frontend/streaming with Claude as the underlying model.
- **Resources:**
  - https://ai-sdk.dev (Vercel AI SDK docs)
  - https://vercel.com/docs/agent-resources/coding-agents/claude-code

### Option C — Go-native loop on the Anthropic Messages API

The Go control plane calls the Anthropic Messages API directly and runs the agent loop
itself (call model → check for tool calls → execute → repeat).

- **Best when:** we want a single-language stack and maximum control with minimal moving
  parts.
- **Trade-off:** highest build cost — we rebuild tools, skills, memory, and session
  persistence ourselves.
- **Resources:**
  - https://platform.claude.com/docs/en/api/overview (Messages API)

**Recommendation lean (not final):** **Option A** preserves the README's stateless-engine
model with the least custom plumbing, because session/memory persistence is a documented
file artifact we move in and out of Supabase. All three options work cleanly with the
**BYOK billing model (§6)**: the user's API key is injected as an environment variable
into the instance. Decide in §9.

---

## 4. Hosting / sandbox — evaluating the founder's set

Where the ephemeral instances run. Scope is the set the founder sent — **E2B, Modal,
Daytona, Railway, Google Cloud, Lambda(s)** — explicitly **not AWS**.

| Platform | Isolation | Cold start | Price (≈) | GPU | Scale-to-zero | Notes |
|---|---|---|---|---|---|---|
| **E2B** | Firecracker microVM, dedicated kernel (strongest) | ~150 ms | $0.0504 / vCPU-hr; Hobby tier $100 credit, 20 concurrent | No | Yes (per-sandbox) | Strongest per-tenant boundary; large catalog of community templates |
| **Daytona** | Container by default; optional Kata/Sysbox ≈ microVM | ~27–90 ms (fastest) | $0.0504 / vCPU-hr | No | Yes | Best when per-turn cold-start latency is the bottleneck |
| **Modal** | gVisor | — | ≈ $0.071 / vCPU-hr; 50k+ concurrent | **Yes (in-sandbox)** | Yes | The only option that can hold a GPU *inside* the sandbox |
| **Railway** | Container PaaS | — | per-service | No | No (per service, not per session) | Great DX; weaker per-session isolation, no true per-session scale-to-zero |
| **Google Cloud — Cloud Run** | gVisor containers, scale-to-zero | — | per-request / CPU-time | Yes (Cloud Run GPU) | Yes | GCP-native baseline; the founder's "google cloud" option |
| **Lambda(s)** | — | — | — | depends | — | **Ambiguous** — clarify which is meant: *AWS Lambda* (excluded, it's AWS) vs *Lambda Labs* (a GPU cloud, not a per-session sandbox). Flagged for the founder. |

**Lean (not final):**

- **E2B** — if "isolation is first-class" (README principle 1) dominates: Firecracker with a
  dedicated kernel per sandbox is the strongest boundary in the set.
- **Daytona** — if a fresh sandbox spins up per turn and cold-start latency dominates the UX.
- **Modal** — only if GPU work happens on the sandbox side (e.g. local embeddings/inference).
- **Cloud Run** — the GCP-native baseline if the founder prefers staying inside Google Cloud.

> **Note on "Lambda(s)":** the term is ambiguous. *AWS Lambda* is excluded by the
> "not AWS" constraint. *Lambda Labs* is a GPU cloud, not a per-session sandbox primitive, so
> it would compete with Modal for GPU workloads rather than with E2B/Daytona for isolation.
> The founder should confirm which was intended.

### 4.1 Warm pool (latency optimization)

Cold starts — even at ~150 ms (E2B) or ~27 ms (Daytona) — compound when a user triggers
multiple agent turns in rapid succession. A **warm pool** of 2–5 pre-provisioned instances
with the Houston runtime pre-loaded (but no tenant state) can eliminate perceived latency:

1. Pool maintains N idle instances with the base image (runtime + tools) already booted.
2. On request, the control plane **claims** one from the pool, injects the scoped token, and
   hydrates only the tenant-specific state (agent definition + credentials + context).
3. After teardown the instance is **scrubbed** (all tenant state wiped) and returned to the
   pool.
4. A background process replenishes the pool to maintain N ready instances.

This trades a small fixed compute cost (N × idle-instance price) for consistently fast
first-response times. The pool size is tunable per deployment and can scale with demand
(more instances during business hours, fewer at night).

---

## 5. Supabase as the hard isolation boundary

Supabase is where the README's *"isolation enforced by the data layer"* becomes concrete.

- **Postgres + RLS** — every row carries `org_id` and `group_id`; Row-Level Security policies
  read those from the request's JWT claims. The visibility rule from the README:

  ```
  org = mine  AND  group ∈ { general, mine }
  ```

  Even a malformed query cannot return another org's rows, because RLS filters before results
  are returned.
- **pgvector** — knowledge embeddings live in Postgres; semantic search is scoped to
  `{ general, group }` of the requesting org by the same RLS policies.
- **Storage buckets** — files / images / scripts stored under per-org path prefixes, with
  storage access policies mirroring the RLS rule.
- **Auth** — issues the identity (`user_id`, `org_id`, `grupo_id`) as JWT / `app_metadata`
  claims that RLS and the control plane both trust.
- **Per-instance scoped token** — each ephemeral instance receives a **short-lived credential
  scoped to exactly one org/group** (the sketch's *"Supabase CLI tied to a user_id"*). A
  compromised instance physically cannot read another tenant, because its token never carries
  another tenant's claims.
- **The leak test** — the README's Milestone 1 proof: seed Org A and Org B, act as Org A, and
  assert that **not a single row** from Org B is returned. This is the acceptance gate for the
  future Milestone-1 code cut.

### 5.1 Context graph — sharing beyond `{ general, mine }`

The base RLS rule (`org = mine AND group ∈ { general, mine }`) covers the common case.
But the founder's requirement includes **controlled sharing between groups** — and
potentially **across organizations** (e.g., an accounting firm sharing a template with a
client org).

A rigid tree hierarchy (org → group → agent) cannot express this. A **context graph**
can:

```sql
CREATE TABLE context_shares (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_type text NOT NULL CHECK (source_type IN ('org', 'group', 'agent')),
  source_id   uuid NOT NULL,
  target_type text NOT NULL CHECK (target_type IN ('org', 'group', 'agent')),
  target_id   uuid NOT NULL,
  permissions text NOT NULL DEFAULT 'read' CHECK (permissions IN ('read', 'write')),
  created_at  timestamptz DEFAULT now(),
  created_by  uuid REFERENCES auth.users(id),
  UNIQUE (source_id, target_id)
);

-- RLS on context_shares itself: only org admins can create/view shares
-- involving their own org's entities.

-- Extended visibility policy on contexts:
CREATE POLICY "context_visibility" ON contexts FOR SELECT USING (
  -- base rule: own org + own group or general
  (org_id = auth.jwt()->>'org_id'
   AND group_id IN (auth.jwt()->>'group_id', 'general'))
  OR
  -- extended: explicit share grants targeting my org, group, or agent
  id IN (
    SELECT source_id FROM context_shares
    WHERE target_id = (auth.jwt()->>'group_id')::uuid
       OR target_id = (auth.jwt()->>'org_id')::uuid
  )
);
```

This keeps the base case simple (no join needed when there are no shares) while
enabling the graph edges the founder asked for. Share rows are themselves RLS-protected
so only admins of the source entity can create them.

### 5.2 Event sourcing — immutable context history

Context mutations today are destructive: `learnings.json` is overwritten,
`context-ledger.json` is replaced. For a multi-tenant platform serving enterprises,
this is insufficient — we need audit trails, rollback, and cross-agent reactivity.

**Every context mutation becomes an immutable event:**

```sql
CREATE TABLE context_events (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  timestamp   timestamptz DEFAULT now(),
  org_id      uuid NOT NULL REFERENCES organizations(id),
  group_id    uuid REFERENCES groups(id),
  agent_id    uuid REFERENCES agents(id),
  event_type  text NOT NULL,
    -- 'context_updated', 'learning_added', 'learning_removed',
    -- 'skill_changed', 'file_added', 'file_removed',
    -- 'integration_connected', 'integration_removed'
  payload     jsonb NOT NULL,       -- the diff / new value
  actor_id    uuid REFERENCES auth.users(id),  -- who or what triggered it
  session_id  uuid                  -- which session produced it (nullable)
);

-- RLS: same org_id + group_id scoping as all other tables
-- Index on (agent_id, timestamp) for efficient state reconstruction
CREATE INDEX idx_context_events_agent ON context_events (agent_id, timestamp);
```

**What this enables:**

| Capability | How |
|---|---|
| **Rollback** | Reconstruct agent state at any point by replaying events up to a timestamp. |
| **Audit trail** | Who changed what, when, in which session. Critical for enterprise compliance. |
| **Cross-agent subscriptions** | Agent B can subscribe to context changes from Agent A via event stream (e.g., a reviewer agent reacts when the bookkeeper agent adds a new learning). |
| **Conflict resolution** | Concurrent sessions on the same agent (§9) can be resolved by merging event streams rather than last-writer-wins on full files. |
| **Debugging** | When an agent behaves unexpectedly, inspect the event log to see what context it was given. |

**Implementation in the hydrate → sync cycle (§7.3):**

- **Hydrate:** the control plane materializes current state by reading the latest snapshot
  (or replaying events since the last snapshot). This becomes the agent's working copy.
- **Sync back:** instead of overwriting state files, the sync step emits events for each
  diff between the pre-session and post-session state. The control plane appends these to
  `context_events`.
- **Snapshots:** periodic materialized snapshots avoid replaying the full event log every
  time. A background job compacts events into snapshots on a configurable interval.

The existing anti-injection sanitization (`learnings_context.rs`) runs in-instance on the
materialized state, unchanged.

---

## 6. Provider accounts & billing — Bring Your Own Key (BYOK)

**Principle.** Houston 2.0 holds **no platform-wide AI API keys** and does not resell tokens.
Each organization/user provides their own **API key** for their chosen provider (Anthropic /
OpenAI / Google), and AI usage is billed by the provider **directly to that API key's
account**. This is the concrete answer to the README's *cost isolation* open question for AI
spend: the billing boundary is the API key itself, per tenant.

This is the same model used by CrewAI, Cursor (BYOK mode), JetBrains, and every major
agent framework that scales without provider-policy risk.

### 6.1 Why BYOK over BYOA (provider-CLI OAuth)

Houston today uses a **BYOA** model: the control plane launches the provider's CLI
(`claude auth login`, `codex login`), captures the OAuth URL, relays it to the user, and
stores the resulting credential file. This works for a desktop app, but introduces
friction and fragility for a multi-tenant cloud platform:

| Concern | BYOA (OAuth relay) | BYOK (API key) |
|---|---|---|
| **Setup UX** | Multi-step: launch CLI → capture URL → user authorizes in browser → paste code back → store credential | One step: paste API key in settings |
| **Credential management** | Refresh tokens expire, need rotation logic, provider can revoke | API keys are long-lived, user manages rotation in their provider dashboard |
| **Provider policy risk** | Tied to consumer subscription; subject to Agent SDK credit limits ($20–$200/mo at API list prices as of June 15, 2026) | Billed directly at API rates with no artificial credit cap; user controls their own spend |
| **Multi-provider support** | Each provider has a different CLI, different OAuth flow, different credential format | Universal: every provider uses `Authorization: Bearer <key>` |
| **Runtime coupling** | Forces Option A (Agent SDK) because the CLI OAuth is what authenticates | Works with all three runtime options (A, B, C) |
| **Stateless principle** | Credential files (`~/.claude/.credentials.json`) must be hydrated/synced as mutable state | A single env var (`ANTHROPIC_API_KEY`) injected at provision time; no state to sync |

**Decision:** BYOK. The user pastes their API key in Houston settings. The control plane
stores it **encrypted in Supabase** (scoped to the tenant) and injects it as an environment
variable into the ephemeral instance at provision time.

### 6.2 How it works

```
User → Houston settings → "Add provider" → pastes API key
                                              │
                    ┌─────────────────────────┘
                    ▼
            Supabase: provider_keys table
            (org_id, provider, encrypted_key, created_at)
            RLS: only own org can read
                    │
                    │  at instance provision time
                    ▼
            Control plane reads key, injects as env var:
              ANTHROPIC_API_KEY=sk-ant-...
              OPENAI_API_KEY=sk-...
              GOOGLE_API_KEY=...
                    │
                    ▼
            Ephemeral instance uses standard SDK auth
            (every SDK reads from env vars by default)
```

### 6.3 Provider support matrix

| Provider | API key source | Env var | Pricing model |
|---|---|---|---|
| **Anthropic** | console.anthropic.com → API keys | `ANTHROPIC_API_KEY` | Per-token, pay-as-you-go |
| **OpenAI** | platform.openai.com → API keys | `OPENAI_API_KEY` | Per-token, pay-as-you-go |
| **Google AI** | aistudio.google.com → API keys | `GOOGLE_API_KEY` | Free tier + pay-as-you-go |
| **OpenRouter** | openrouter.ai → API keys | `OPENROUTER_API_KEY` | Per-token, 100+ models |
| **Custom / self-hosted** | User's own endpoint | `OPENAI_API_BASE` + key | User-managed |

### 6.4 Provider-agnostic runtime

BYOK naturally enables **model-agnostic operation**. The control plane does not care which
provider the key belongs to — it injects the appropriate env var(s) and the runtime SDK
resolves the rest. This means:

- A user can switch from Claude to GPT to Gemini by changing their API key in settings —
  no agent redefinition needed.
- Different agents within the same org can use different providers (e.g., Claude for coding
  tasks, Gemini for research, a local model via OpenRouter for sensitive data).
- Houston is **decoupled from any single provider's pricing policy**, terms of service, or
  credit system changes.

### 6.5 Interaction with §3 (runtime options)

Unlike BYOA (which forced Option A because it depended on the provider CLI's OAuth flow),
BYOK works cleanly with **all three runtime options**:

- **Option A (Agent SDK):** set `ANTHROPIC_API_KEY` env var; the SDK authenticates via key
  instead of OAuth. Documented and supported.
- **Option B (Vercel AI SDK):** pass the key to the provider constructor
  (`createAnthropic({ apiKey })`) — this is the primary auth method.
- **Option C (Go-native):** set the `x-api-key` header on Messages API calls directly.

This **removes runtime coupling from the billing decision** and lets us pick the runtime
purely on technical merit.

### 6.6 Security

- API keys are **encrypted at rest** in Supabase using `pgsodium` / Vault.
- Keys are **never logged**, never included in error responses, never sent to the client
  after initial submission.
- Each key is scoped to one org via RLS — a compromised instance cannot read another
  tenant's keys.
- The control plane injects keys as **env vars into the ephemeral instance**, which is
  destroyed after use — keys never persist on disk beyond the instance lifetime.
- Users can **rotate or revoke** keys at any time from their provider's dashboard; Houston
  re-reads the stored key on each provision.

---

## 7. Agent lifecycle — creation & personalization on Houston 2.0

In Houston today an agent **is a folder on local disk**, and the filesystem is the source of
truth: `agents_crud::create()` materializes a folder (`.houston/agent.json`, `CLAUDE.md`,
seeds, skills, skeleton via `seed_agent()`), and personalization accumulates as **state files**
that `build_agent_context()` re-injects into the prompt at every `sessions::start`.

Houston 2.0 cannot keep the local filesystem as the source of truth — instances are ephemeral
and multi-tenant. The reframe (which is exactly the README's *"an agent is a configuration, not
a process"*):

> **Supabase is the source of truth; the instance's disk is a materialized, throwaway working
> copy.** Creating an agent = persisting its definition to Supabase. Running a session =
> *hydrating* that definition into the instance, running Houston's existing filesystem logic
> unchanged, and *syncing back* the mutated state. Personalization lives in Supabase, not on a
> disk that disappears when the instance tears down.

The payoff: **Houston's in-instance agent logic is reused as-is** (`seed_agent`,
`build_agent_context`, the learnings/skills assembly). We only add a hydrate-before /
sync-after wrapper at the control-plane boundary (§2).

### 7.1 Artifact mapping (disk → Supabase)

| Houston artifact | Nature | Houston 2.0 home |
|---|---|---|
| `.houston/agent.json` (id, config_id, color, ts) | definition | `agents` row + `org_id`, `group_id` |
| `CLAUDE.md` — instructions | definition | agent definition (Postgres column or Storage object) |
| `CLAUDE.md` — `## Learnings` section | mutable state | synced back each session (via context_events §5.2) |
| `.agents/skills/` | versioned definition | Supabase Storage, per-org prefix (or a `skills` table) |
| seeds: `outputs.json`, `routines.json`, … | initial state | written at creation, then mutable |
| `learnings.json`, `integrations.json` | mutable state | per-agent state, hydrated/synced (changes tracked as events §5.2) |
| `config/context-ledger.json` ("what the agent knows about you") | mutable knowledge | candidate for the org/group knowledge layer + pgvector (§5) |
| `activity.json`, `routine_runs` | append-only log | per-agent state table (naturally event-sourced) |
| `agent-schemas` JSON Schemas | static contract | embedded at build — **not** per-tenant |
| `AGENTS.md` / `GEMINI.md` symlinks | derived | recreated in-instance by `seed_agent()` — not stored |

### 7.2 Creation — the four paths still converge

In Houston, blank / "create with AI" / store / GitHub all converge on `create()`. In Houston
2.0 they converge on **"persist the agent definition to Supabase"** (scoped to org + group):

- **AI-assist** (`generate_instructions.rs`, cheap model, 60 s one-shot) runs in the control
  plane or a short-lived instance **using the tenant's BYOK API key (§6)** — so even
  agent-authoring generation is billed to the user's own account.
- **Store / GitHub install** fetches `houston.json` + `CLAUDE.md` + `icon.png` + skills,
  validates `id`, and stores those as Supabase objects scoped to the org (replacing Houston's
  "installed directory on disk").
- The result of every path is a definition record, never a long-lived process.

### 7.3 Session & personalization — hydrate → run → sync back

1. Control plane resolves tenant context (org + group) and mints the scoped token (§5).
2. Provisions an instance and **hydrates** the agent folder from Supabase **and** injects the
   BYOK API key as env var (§6).
3. **In-instance, unchanged Houston logic:** `seed_agent()` (idempotent) →
   `build_agent_context()` assembles the prompt (Working Directory, mode file, learnings,
   skills index, workspace context, used integrations).
4. Runs the session via the chosen runtime (§3).
5. **Syncs back** the mutated state by emitting **context events** (§5.2) for each diff
   between pre-session and post-session state (`learnings.json`, `integrations.json`,
   `context-ledger.json`, `CLAUDE.md ## Learnings`, routines/activity).
6. Tears down (scale to zero).

The learnings **anti-injection** sanitization (`learnings_context.rs`: drop "ignore previous
instructions"-style entries, strip control chars, cap ~4 000 chars, mark as background data)
runs in-instance and composes with our tenant isolation — keep it as-is.

### 7.4 What changes vs Houston

- **Source of truth:** local disk → Supabase; the instance disk is an ephemeral cache.
- **Multi-tenancy:** every agent carries `org_id` + `group_id`; RLS (§5) enforces isolation.
- **Workspace:** in Houston just a folder grouping agents; here it becomes a tenant-scoped
  grouping — a natural candidate to map onto the README's **group**, with "workspace context"
  → group knowledge and `context-ledger` → general/group knowledge in pgvector (§5).
- **State mutations:** tracked as events (§5.2) instead of destructive overwrites, enabling
  rollback, audit trails, and concurrent-session conflict resolution.
- **Billing:** BYOK API key (§6) instead of BYOA OAuth relay; simpler, provider-agnostic,
  and decoupled from consumer subscription policies.

---

## 8. Comparison — Houston 1 vs Houston 2.0

| Aspect | Houston 1 (always-on) | Houston 2.0 (this design) |
|---|---|---|
| **Isolation** | By VPS (strong but expensive) | By RLS + scoped tokens + ephemeral instances |
| **Cost per idle org** | ~$5–15/mo (VPS running) | ~$0 (scale-to-zero) |
| **Scalability** | Linear: n orgs = n VPS | Elastic: shared compute, on-demand |
| **Context sharing** | Not supported (VPS-isolated) | Native via context graph (§5.1) |
| **Change history** | None (destructive file writes) | Event sourcing (§5.2) with rollback |
| **Provider coupling** | Tied to Claude Code OAuth | Provider-agnostic BYOK (§6) |
| **State management** | Local filesystem | Supabase (hydrate/sync cycle) |
| **Complexity** | Low (Docker + filesystem) | Medium (worth the trade-off at scale) |

---

## 9. Open decisions / next steps

- **Runtime pick** (§3): A / B / C.
- **Hosting pick** (§4): E2B / Daytona / Modal / Cloud Run / Railway — and resolve the
  "Lambda(s)" ambiguity.
- **Cost isolation** (README open question): **AI spend is now resolved by BYOK** (§6 — billed
  to each tenant's own API key account). What remains open is *compute* cost isolation: is
  shared ephemeral compute enough, or do some tenants need dedicated resources?
- **Groups** (README open question): a controlled list per org, or free-form labels?
- **Context sharing scope** (§5.1): is cross-org sharing a Milestone 1 requirement, or can
  the context graph be deferred to Milestone 2 (start with intra-org `{ general, mine }`)?
- **Event sourcing granularity** (§5.2): full event sourcing from day one, or start with
  append-only activity log and add full event sourcing in a later milestone?
- **Agent storage shape** (§7): mirror the agent folder as opaque Storage objects (fast port,
  least logic) vs. normalize into Postgres tables (queryable, RLS-granular, more mapping work).
  Likely hybrid: definition + state in Postgres, large/opaque blobs (skills, icons) in Storage.
- **Sync timing** (§7): persist state at end-of-session vs. write-through on every change
  (durability vs. chattiness).
- **Session concurrency** (§7): Houston assumes a single writer per agent folder; multi-tenant
  may run concurrent sessions of one agent → need a strategy (single active-session lock,
  last-writer-wins, or session forking). Event sourcing (§5.2) enables merge-based resolution
  as an alternative to locking.
- **Warm pool sizing** (§4.1): how many pre-provisioned instances to maintain, and whether to
  scale the pool dynamically based on time-of-day or demand signals.
- **Next branch after this doc:** Milestone 1 scaffold — Go control-plane skeleton + Supabase
  schema + RLS policies + the leak test. Explicitly **out of scope** for this `develop` cut
  (design doc only).

---

## 10. Resources

**Reference implementation (how Houston does it today)**
- Houston (gethouston) — https://github.com/gethouston/houston
  — provider connect/relay: `engine/houston-engine-core/src/provider/login_relay.rs`,
  adapters `engine/houston-terminal-manager/src/provider/{anthropic,openai,gemini}.rs`,
  REST `engine/houston-engine-server/src/routes/providers.rs`, frontend
  `app/src/components/shell/provider-login-dialog.tsx`.

**Runtime**
- Claude Code headless mode — https://code.claude.com/docs/en/headless
- Claude Agent SDK — sessions — https://platform.claude.com/docs/en/agent-sdk/sessions
- Claude Agent SDK — hosting — https://platform.claude.com/docs/en/agent-sdk/hosting
- Vercel AI SDK — https://ai-sdk.dev
- Vercel × Claude Code/Agent SDK — https://vercel.com/docs/agent-resources/coding-agents/claude-code
- Anthropic Messages API — https://platform.claude.com/docs/en/api/overview

**Hosting / sandbox**
- Daytona vs E2B vs Modal vs Vercel Sandbox (2026) — https://www.startuphub.ai/ai-news/artificial-intelligence/2026/daytona-vs-e2b-vs-modal-vs-vercel-sandbox-2026
- AI code sandbox benchmark 2026 — https://www.superagent.sh/blog/ai-code-sandbox-benchmark-2026
- Daytona vs E2B — https://northflank.com/blog/daytona-vs-e2b-ai-code-execution-sandboxes
- AI sandbox pricing comparison — https://northflank.com/blog/ai-sandbox-pricing
- Best agent cloud platforms 2026 — https://northflank.com/blog/best-agent-cloud-platforms

**Supabase**
- Row Level Security — https://supabase.com/docs/guides/database/postgres/row-level-security
- pgvector / AI & vectors — https://supabase.com/docs/guides/ai
- Storage access control — https://supabase.com/docs/guides/storage/security/access-control

**Industry context**
- CrewAI LLM connections (BYOK model) — https://docs.crewai.com/en/learn/llm-connections
- Anthropic Agent SDK credit split (June 15, 2026) — affects BYOA model, motivates BYOK
- OpenAI API vs ChatGPT subscription separation — API keys are independent of consumer plans

> All hosting/runtime figures above are from May 2026 web research and should be re-verified
> before any final platform commitment — pricing in particular drifts.

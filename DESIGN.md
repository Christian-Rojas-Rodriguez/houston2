# Houston 2.0 — Design (HOW)

> `README.md` describes **what** we are building and **why**. This document describes **how**:
> the runtime model, the hosting/sandbox candidates, and how Supabase enforces isolation.
> It is a decision-support document — the final runtime and hosting picks are still open
> (see §6).

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
file artifact we move in and out of Supabase. Decide in §6.

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

---

## 6. Open decisions / next steps

- **Runtime pick** (§3): A / B / C.
- **Hosting pick** (§4): E2B / Daytona / Modal / Cloud Run / Railway — and resolve the
  "Lambda(s)" ambiguity.
- **Cost isolation** (README open question): is shared ephemeral compute enough, or do some
  tenants need dedicated resources?
- **Groups** (README open question): a controlled list per org, or free-form labels?
- **Next branch after this doc:** Milestone 1 scaffold — Go control-plane skeleton + Supabase
  schema + RLS policies + the leak test. Explicitly **out of scope** for this `develop` cut
  (design doc only).

---

## 7. Resources

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

> All hosting/runtime figures above are from May 2026 web research and should be re-verified
> before any final platform commitment — pricing in particular drifts.

# Houston 2.0 — Architecture (What & Why)

> A platform to host AI agents in a multi-tenant setup. A single orchestrator serves 1..N
> agents for multiple organizations, guaranteeing that no organization can access another's
> data, while agents within the same organization share a structured common knowledge base.
>
> This document is the **what** and the **why**. The **how** lives in [`DESIGN.md`](./DESIGN.md).

---

## What we're building (MVP first)

The first deliverable is an **isolation demo**: the platform running for multiple
organizations and groups at once, proving that tenants are fully isolated and showing how the
design generalizes to **N orgs / N groups / N users**.

**First cut:** 2 organizations · 2 groups · 3 users.

Two tenant-isolated use cases are in scope:

1. **Create an agent** — blank, from a shared **template** (e.g. a sales agent), AI-assisted,
   or imported from GitHub. Every new agent is written scoped to the creator's org + group,
   isolated from other tenants from birth.
2. **Run an agent** — a request (prompt + optional files) is answered by an agent that only
   ever sees its own org + group context.

For the MVP this runs **locally** (no VPS): a **Go orchestrator**, **Supabase** (Postgres +
Storage, both with Row-Level Security) as the data layer, and **Claude Code in the local
terminal** as the runtime. See [`DESIGN.md`](./DESIGN.md) for the flows and diagrams.

---

## Why

1. **Isolation is first-class, not an afterthought.** Every piece of data is born associated
   with an organization and a group. There is no ownerless data.
2. **Isolation is enforced by the data layer, not just the application.** Even if a bug lets a
   malformed query through, the storage layer must not return another organization's data.
   RLS in Postgres **and** Storage is the hard guarantee.
3. **An agent is a configuration, not a process.** Creating an agent means registering a
   definition in Supabase, not spinning up a machine.
4. **Shared knowledge is explicit.** Common knowledge is a declared category (`general` /
   group) with clear rules about who can read it.
5. **The MVP proves it.** Success = a test that fails the moment isolation breaks (see below).

---

## High-level picture

```mermaid
flowchart TB
    subgraph T["Tenants (MVP: 2 orgs · 2 groups · 3 users)"]
      U1["User A — Org 1 / Group 1"]
      U2["User B — Org 1 / Group 2"]
      U3["User C — Org 2 / Group 1"]
    end
    O(("Orchestrator (Go)"))
    R["Claude Code — local runtime (MVP)"]
    DB[("Supabase — Postgres RLS + Storage buckets RLS")]
    U1 -->|request / response| O
    U2 -->|request / response| O
    U3 -->|request / response| O
    O <-->|prompt · context| DB
    O <-->|upload · context · inference| R
```

---

## Domain model

| Concept | Description |
|---|---|
| **Organization** | The tenant. The unit of isolation. |
| **Group** | A subdivision within an org. There is always a special `general` group plus any number of functional groups (e.g. `developers`, `sales`). |
| **Agent** | An agent instance: identity, instructions, configuration. Belongs to one org and one group. |
| **Template** | A reusable agent blueprint in a **shared, read-only catalog** (e.g. a sales agent). Instantiated *into* an org/group, where the resulting agent is tenant-isolated. |
| **Knowledge / files** | Documents, skills, scripts associated with an org + group, stored in Supabase Storage buckets. |
| **Membership** | The relationship between a user and the org + group they belong to. Defines what they can see. |
| **Prompt / Conversation** | A request and its interaction history with an agent. |

---

## The isolation model

The central rule of the entire system:

```
An agent in group G, inside organization O, can read data where:

    organization = O   AND   group ∈ { general, G }

It can never read anything from another organization.
```

- **Across organizations:** total isolation. No exceptions, no admin mode that crosses the
  boundary.
- **Within an organization:** `general` knowledge is visible to all agents in the org; group
  knowledge is visible only to that group; two groups cannot see each other (except via
  `general`).
- **Templates** are the one shared, cross-tenant artifact — but they are **read-only**, and
  once instantiated the resulting agent is fully org/group-scoped.

---

## MVP scope & the leak test

**Scope:** 2 organizations, 2 groups, 3 users.

**The proof — the leak test.** Seed data for Org A and Org B (with their groups), act as a
user in Org A / Group 1, and assert that **not a single row and not a single Storage object**
from Org B — or from another non-`general` group of Org A — is returned. If isolation breaks,
this test fails. This is the acceptance gate for the MVP.

---

## Roadmap

- **Milestone 1 — the isolation cut (MVP, local).** Data model with org + group on every unit;
  RLS in Postgres and Storage; create-agent and run-agent flows; the leak test passes. Runs
  locally with the Go orchestrator + Supabase + Claude Code.
- **Post-MVP — operations & scale.** Managed/ephemeral cloud hosting, retrieval (pgvector
  semantic search), per-tenant observability and usage limits, horizontal scale. Options and
  research are recorded in the [`DESIGN.md`](./DESIGN.md) appendix.

---

## Out of scope (for now)

- A specific end-user interface (desktop, mobile, web).
- Real billing integration (provider accounts cover AI spend; see the `DESIGN.md` appendix on
  BYOA).
- Cloud/ephemeral hosting and hardware-level isolation (post-MVP).
- Retrieval optimization / semantic search — the MVP sends all org+group context.

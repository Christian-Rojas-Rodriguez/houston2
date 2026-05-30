# Houston 2.0 — Architecture

> A platform to host AI agents in a multi-tenant setup. A single host runs 1 to N agents
> for multiple organizations, guaranteeing that no organization can access another's data,
> while agents within the same organization share a structured common knowledge base.
>
> This document describes **what** we are building and **why**.
> The **how** (language, database, services) is decided separately.

---

## The Problem

We need a platform that:

- Hosts **1 to N agents** on a shared infrastructure.
- Serves **multiple organizations** from the same host.
- Guarantees that **two organizations never share information**, by any means.
- Allows agents **within** the same organization to share knowledge:
  - a **general** knowledge layer accessible to all agents in the org,
  - and a **group-level** knowledge layer accessible only to agents in that group.

### Concrete example

```
Host
├── Organization A
│   ├── General knowledge          → readable by all agents in A
│   ├── Group: Developers (5)      → share knowledge among themselves
│   └── Group: HR (5)              → share knowledge among themselves
│
├── Organization B
│   ├── General knowledge
│   ├── Group: X (5)
│   └── Group: Y (5)
│
└── ... up to N organizations, M agents total
```

---

## Design Principles

1. **Isolation is first-class, not an afterthought.** Every piece of data is born
   associated with an organization. There is no concept of ownerless data.
2. **Isolation is enforced by the data layer, not the application layer.** Even if a bug
   lets a malformed query through, the storage layer must not return another
   organization's data. The application is the first line of defense; the data layer is
   the hard guarantee.
3. **An agent is a configuration, not a process.** Creating an agent means registering a
   definition, not spinning up a machine. A single engine serves all agents.
4. **The engine is stateless.** All state lives in storage. This allows running multiple
   engine copies behind a load balancer without coordination.
5. **Shared knowledge is explicit.** Common knowledge is not a side effect — it is a
   declared category (`general` / group) with clear rules about who can read it.

---

## Domain Model

| Concept | Description |
|---|---|
| **Organization** | The tenant. The unit of isolation. |
| **Group** | A subdivision within an org. There is always a special `general` group and any number of functional groups (e.g. `developers`, `hr`). |
| **Agent** | An agent instance: its identity, instructions, and configuration. Belongs to one org and one group. |
| **Knowledge** | A unit of information (document, note) associated with an org and a group, retrievable by semantic search. |
| **Conversation** | The interaction history with an agent. |
| **Membership** | The relationship between a user and the org + group they belong to. Defines what they can see. |
| **Usage record** | How much each organization consumed, for measurement and billing. |

---

## The Isolation Model

This is the central rule of the entire system:

```
An agent in group G, inside organization O, can read knowledge where:

    organization = O   AND   group ∈ { general, G }

It can never read anything from another organization.
```

From this rule:

- **Across organizations:** total isolation. No exceptions, no admin mode that crosses the
  boundary, no endpoint that joins data from multiple orgs.
- **Within an organization:**
  - `general` knowledge is visible to all agents in the org,
  - group knowledge is visible only to agents in that group,
  - two different groups cannot see each other (except through `general`).

**Defense in depth.** Isolation is applied at two layers:

1. **Data layer:** storage only returns rows that belong to the requesting organization
   and group. This is the hard guarantee.
2. **Application layer:** every request carries a verified identity that fixes the
   context (organization + group) before any data is touched. This is the first barrier.

> The proof that the system works is a test that fails if isolation breaks:
> seed data for Org A and Org B, act as Org A, and assert that **not a single row**
> from Org B is returned.

---

## Knowledge Hierarchy

```
        ┌─────────────────────────────────────────┐
        │   GENERAL knowledge — Organization A     │
        │   (readable by all agents in the org)    │
        └───────────────────┬─────────────────────┘
                            │  visible downward
              ┌─────────────┴──────────────┐
              │                            │
    ┌─────────▼──────────┐      ┌──────────▼─────────┐
    │  Group: Developers  │      │  Group: HR          │
    │  own knowledge      │  ✕   │  own knowledge      │
    │                     │      │                     │
    └─────────────────────┘      └─────────────────────┘
```

Each agent, when querying, accesses the union of **the org's general knowledge** plus
**its own group's knowledge**. Nothing more.

---

## Agent Model

An agent is not a server or a container. It is a **definition**: identity, instructions,
and configuration. The **engine** — a single always-on service — reads those definitions
and handles all agent conversations concurrently.

Consequences:

- **Creating an agent** = registering its definition. The engine serves it without restarting.
- **One hundred agents** are not one hundred processes: they are one hundred definitions
  that a single engine handles.
- **Compute cost** only appears when an agent receives a request.
- **Scaling** = running more engine copies behind a load balancer, since all state lives
  in shared storage.

---

## Query Flow (conceptual)

```
1. A request arrives for an agent, carrying a verifiable identity.
2. The system validates the identity and derives its context: organization + group.
3. That context is fixed before any data access.
4. Relevant knowledge is retrieved by semantic search,
   scoped to { general + group } of that organization.
5. The agent input is assembled: instructions + retrieved knowledge + conversation history.
6. A response is generated.
7. Usage is recorded and the exchange is saved.
8. The response is returned.

At no point in this flow can one organization's context leak into another's.
```

---

## Improvements Over the Previous Approach

- **Multi-tenancy from day one**, not bolted onto a single-user foundation.
- **Isolation guaranteed by the data layer**, not trusted to application code alone.
- **Shared knowledge as an explicit category** (general / group) with clear rules.
- **Stateless engine** → simple horizontal scaling.
- **Usage measurement per organization** built in from the start.
- **Core decoupled from any interface**: the engine is driven by an API; clients
  (web, desktop, anything) are built on top, not inside.

---

## Roadmap

**Milestone 1 — The isolation cut.**
The data model skeleton with organization and group on every unit, isolation enforced
at the data layer, and identity-based context on every request.
Deliverable: the leak test passes (Org A never sees Org B).

**Milestone 2 — Knowledge, agents, and conversation.**
Knowledge ingestion and semantic search respecting the general/group hierarchy.
Agent creation. The full end-to-end query flow. Usage recording per org.
Load validation with the target scenario.

**Milestone 3 — Operations and scale.**
Organization and group provisioning. Per-tenant observability. Usage limits per org.
Multiple engine copies behind a load balancer.

---

## Open Decisions (implementation phase)

These are answered when we choose the technology:

- **Identity:** how are agent/user identities issued and verified?
- **Storage:** which database engine, and how does it enforce row-level isolation?
- **Semantic search:** how are knowledge representations generated and stored?
- **Generation:** which model provider, and how is it abstracted for replaceability?
- **Cost isolation:** is measuring usage per org enough, or do some tenants need dedicated resources?
- **Groups:** are they a controlled list per organization, or free-form labels?

---

## Out of Scope (for now)

- Any specific user interface (desktop, mobile, web).
- Real billing integration (usage is measured, not charged).
- Hardware-level isolation per organization (a future scaling option).
- An agent catalog or marketplace.

---

## Summary

```
         Verified identity (org + group)
                      │
                      ▼
            ┌─────────────────┐
            │     Engine       │   one service, stateless,
            │  (serves N       │   handles all agents concurrently
            │   agents)        │
            └────────┬─────────┘
                     │  fixes tenant context
                     ▼
            ┌─────────────────┐
            │    Storage       │   row-level isolation:
            │  organization    │   org   = mine
            │  + group         │   group ∈ { general, mine }
            └─────────────────┘
```

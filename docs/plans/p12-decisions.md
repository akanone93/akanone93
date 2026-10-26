<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P12 AI Generation — Decisions

> **Status:** Decision record, built up question by question in a preparation session with the
> maintainer (started 2026-10-26), before P12 starts. Binding input for the P12 plan, its ADRs
> (ADR-0017 first) and every P12 session. Nothing here is implemented yet.

- **Purpose:** settle the technical questions of P12 up front, so the implementing agent starts
  from decisions instead of reopening them.
- **Scope:** spec §5.1 (P12), §23 (`AI-001` to `AI-060`), `I18N-013`, `SEC-191`.
- **Not yet applied:** no existing document was changed. The follow-ups listed with each
  decision are made in the pull requests that need them, with the maintainer's approval where
  `AGENTS.md` §7 requires it.

---

## Decisions

### D-1. Where the AI backend runs

- **Decision:** AI is an **AI component inside `plux-server`**, deployable as its **own role**
  next to `api`, `worker` and `fnrunner` (ADR-0006). Single-node installations run it in the
  same process as the other roles; SaaS installations scale it independently.
- **Reason:**
  - AI requests are I/O-bound. The server spends milliseconds on context assembly (`AI-004`),
    redaction (`AI-040`) and validation (`AI-002`), then waits seconds on the provider.
    Hundreds of requests per second become thousands of mostly idle streams, which the existing
    server handles cheaply. Throughput is bounded by provider capacity, cost and rate limits,
    controlled through quotas and queueing (`AI-041`), not by the server.
  - `AI-002` requires the **same** validators as human edits. Running in-process calls the
    compiler's validators directly on every attempt of the repair loop (`AI-003`), with no
    second copy and no network hop.
  - `SRV-001` and ADR-0006 already define one binary with deployable roles; a separate role
    gives independent scaling and blast radius (a slow provider cannot starve normal API
    traffic) without a second service to build, deploy and observe.
- **Rejected:** a separate AI service outside `plux-server`; it would duplicate or remotely
  call the validators and add a second deployable for no measurable gain.
- **Follow-ups:**
  - `SRV-001` names the roles `api`, `worker` and `fnrunner`; adding the AI role changes its
    wording (spec change, `AGENTS.md` §3) and ADR-0006's role table.
  - What the role holds (database access for context, signing keys: none), how it reaches
    providers, and its name are settled in later decisions of this file.

### D-2. What the AI role holds and how it reaches providers

- **Decision:** the AI role holds **database access** (read context for `AI-004`; write
  proposals, usage and audit records) and **no signing keys**. Provider credentials are stored
  under envelope encryption and unwrapped only at call time (`SEC-106`). Every provider call
  goes through the existing **`egress` role**, which allows only the configured provider hosts.
- **Private addresses:** `egress` refuses private addresses by default. Only providers
  configured by an **installation administrator** may point at private addresses (a local
  Ollama, vLLM or LM Studio). Providers configured by an **organisation** must resolve to
  public addresses. The organisation setting "local providers only" (`AI-040`) is enforced by
  the same rule.
- **Reason:** reuses the isolation the server already has (L-3, L-4, `FN-017`) instead of
  inventing a new one; an organisation cannot turn a provider URL into server-side request
  forgery against the internal network.
- **Rejected:** direct outbound calls from the AI role (no allowlist, SSRF risk); a role with
  no database like `fnrunner` (every context lookup and repair attempt becomes extra round trips
  through `api`).
- **Follow-ups:** a test that asserts the AI role's dependency set (no signer), as ADR-0006 does
  for the other roles; the egress allowlist source for AI providers.

### D-3. How an AI request runs

- **Decision:** **hybrid job model.** `Start` enqueues a job and returns its ID; the AI role
  takes jobs from a database-backed queue with per-organisation and per-user concurrency limits
  (`AI-041`); clients follow progress through a server-streaming `Watch` call that streams
  partial output (`AI-006`). The finished output is stored as a **proposal record** that the
  user accepts or rejects in whole or per change (`AI-005`) and that survives a closed tab or a
  dropped connection. Studio, the CLI (`plux ai`) and the MCP server (`AI-060`) use the same path.
- **Reason:** gives streaming, fairness between tenants and reliability at once; the queue and
  the stored proposal are what quotas, audit and review-before-apply need anyway.
- **Rejected:** one long synchronous streaming call (work lost on disconnect, weak fairness);
  polling-only jobs (no progressive canvas, `AI-006`).

### D-4. What the model writes

- **Decision:** the model returns a list of **edit operations** against node IDs: insert
  subtree, replace node, set property, delete node (exact set fixed in ADR-0017). A new page is
  one "insert root" operation. Subtrees inside an operation are **real document JSON**; there is
  no second authoring format.
- **Reason:** edits touch only the nodes they name (`AI-010` edit selection), each operation is
  one reviewable change (`AI-005` per-change accept or reject), and operations apply as they
  arrive (`AI-006` streaming). Node IDs the model references are validated like everything else.
- **Rejected:** full document JSON for every request (slow and costly on large pages, weak
  models damage untouched parts, per-change review needs a diff after the fact); a compact
  authoring format expanded by the server (a second format to keep in step with the schema;
  revisit only if the evaluation suite shows free models need it).

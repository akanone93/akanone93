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

### D-5. How the model gets the context it needs (progressive disclosure)

- **Decision:** every generation is a short conversation with the model, built up in rounds:
  1. **First request, complete frame.** It carries everything the model needs to do the job
     except the full item details:
     - the **response format**: the reply envelope (`kind`: `need` or `final`), the edit
       operations of D-4, and the **recursive structure** of a node, an action graph and every
       other shape the answer may contain, with the rules for IDs, bindings and references;
     - the **index of everything**: every widget and every action in one line each;
     - the organisation's components, templates, design tokens, data sources, state schemas,
       translation keys and functions (`AI-004`), with secrets and `sensitive` fields removed
       (`AI-040`);
     - the user's prompt and, for an edit, the current selection.
  2. **The model asks.** It replies `need` with the items it wants in full: widgets, actions,
     types, components and so on.
  3. **The server answers** with the full details of exactly those items, including the types
     and enums they reference. The model replies either `need` again or `final` with the edit
     operations carrying the whole result: layout, properties, actions, state and content.
  4. **Validation and repair.** The final answer goes through the full validators; on failure,
     the repair loop (`AI-003`) sends the diagnostics back together with the details of every
     item they mention. A valid answer becomes a proposal (D-3).
- **Rules:**
  - The envelope is plain JSON with a `kind` discriminator, so the same loop works with strict
    structured output and with JSON mode (`AI-002`); it does not depend on native tool calling.
  - Strict output for the `final` reply is checked against a schema the server builds from the
    items the model requested, so properties are checked precisely and the model, not the
    server, chooses the widgets.
  - The number of `need` rounds is capped by a registry limit (`LIM-001`).
  - The conversation only grows by appending; the first request is identical for the same
    runtime version and organisation context, so provider prompt caching makes later rounds and
    later jobs cheap.
- **Measured (2026-10-28):** index of all 207 widgets ~4.7k tokens; full details of all
  widgets ~58k, types and enums ~12k, all 58 actions ~6k. Sending everything in full is about
  76k tokens plus organisation context, which excludes 32k-context local models.
- **Rejected:** everything in full in every request (size and cost); the server picking
  widgets by relevance or similarity search (the server guesses for the model and narrows the
  design); fine-tuning a model on the schema (cost, and ties Plux to one model, against
  `AI-001`).

### D-6. What each configured model can do (capability profile)

- **Decision:** when an administrator submits a model (base URL, model ID, credential), Studio
  shows a loading state with a message while the server sends a few small **test calls** to it:
  strict JSON Schema output, a recursive schema, JSON mode, one image input, prompt-cache
  reporting. The administrator then moves to the next settings screen with every **detected**
  capability filled in and every **undetected** setting (for example the context window)
  **left empty** until the administrator fills it. Plux keeps no built-in table of known models
  and never pre-fills a guess. The result is the model's **capability profile**; the test can be
  re-run from Admin at any time.
- **Use of the profile:** strict or JSON mode for D-5; whether the first request fits the
  context window; Studio hides features the model cannot do (no screenshot input without
  image support, `AI-011`).
- **Also decided:**
  - **Task routing:** an organisation may assign a different configured model per task, for
    example generation on one model and translations and mock data on a cheaper one.
  - **Fallback:** a second configured provider may take over when the first is down or rate
    limited; the switch is recorded in the audit record of the request (`AI-040`).
- **Rejected:** a capability form filled in by hand (often wrong); a built-in table of common
  models (goes stale, and a guess is never shown as a fact).

### D-7. Evaluation suite and the 1.0 gate

- **Decision:** two tiers.
  1. **Every pull request: replayed model answers.** Real responses are recorded once and
     replayed, so the whole pipeline (protocol, `need` rounds, validation, repair, proposals) is
     tested hermetically and deterministically. Requirement IDs get their tests here
     (`QA-070`).
  2. **Nightly, live:** `make ai-eval` runs the 100+ prompts (`AI-050`) against **DeepSeek V4
     Flash**, the only model run in CI, through its OpenAI-compatible API. The API key is a CI
     secret; the exact model ID is pinned. "Publishable" means the output passes the publish
     pipeline checks short of signing and storing. Results per prompt (pass, repair attempts,
     tokens, time, prompt template version, `AI-051`) are kept as a workflow artifact and
     appended to a history file on a dedicated `ai-eval-history` branch, not on `main`.
- **Manual tests, documented with the other manual tests:** a local deployment (Ollama or a
  self-hosted open-weight model) and Claude. Neither is a CI gate; local is lowest priority.
- **Pending maintainer confirmation:** whether DeepSeek V4 Flash satisfies "free provider" in
  `AI-050` and `AI-007` (see the open question at the end of this file).

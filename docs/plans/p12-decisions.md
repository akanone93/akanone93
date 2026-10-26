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

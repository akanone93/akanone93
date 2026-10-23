<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Rollouts and Experiments Guide

How a release reaches your devices in stages under a health gate, how feature flags change
behaviour without a release, and how an A/B experiment is set up, analysed and promoted. The
requirements are `REL-050` to `REL-053`, `ABT-001` to `ABT-007`, and
[ADR-0078](../adr/0078-rollouts-and-health-gates.md) and
[ADR-0079](../adr/0079-flags-and-experiments.md).

## 1. The idea in one page

- A **rollout** offers a candidate release to a share of a channel's devices, in stages, and
  watches it with a **health gate**. A regression pauses the rollout, or rolls it back.
- A **flag** is a typed value per environment, changed without a release.
- An **experiment** gives different devices different variants of a page, component, prop, flow
  or plugin version, and measures a metric for each.

All three reach devices through the signed control document of their channel
([device trust guide](device-trust.md)), so a change is picked up at the device's next sync, or
at once when silent push is enabled (`SYN-060`).

## 2. Bucketing

Every decision that splits devices uses one rule, so a device's place is stable and monotonic.

- A device's bucket is `sha256(salt || subject) mod 10000`, with the salt of the rollout or
  experiment.
- The subject is the user identifier the host app set (sent hashed), or the device identifier
  when there is none.
- Because the bucket depends only on the salt and the subject, a device that is in at 10% is in
  at 50%. A rollout's share never decreases while it is in progress.

## 3. Rollouts

### Targeting

A rollout can name a targeting rule and segments. The server evaluates them when the device asks
for its manifest, using: build, runtime version, operating system and its version, device class,
locale, region (from the device's setting, never its IP address), channel, and user attributes.

- A user attribute leaves the device only as a hash, and a condition on one supports equality and
  membership only.
- An explicit allow list and deny list name devices, for testers. A denied device is excluded;
  an allowed one is included.
- An invalid rule is refused with `PLX-3152`.

### Stages and schedule

A rollout has stages, each with a share and a duration, and optionally a start and an end time.
A scheduler on the elected leader moves active rollouts through their stages, ends the ones past
their end time, and evaluates the health gate. Every step is a conditional update, so overlapping
schedulers do each step once.

- At most one rollout of a channel is in progress at any moment (`PLX-3153`).
- An app may have at most `rollouts.activePerApp` active rollouts (default 10).
- Ending a rollout promotes the candidate, or reverts by creating a new release that repeats the
  previous one. Anti-rollback is never relaxed (`REL-006`), so a revert is a new, higher release.

### Health gate

Between two runs of the gate, the candidate and the previous release are compared over a window
and a minimum sample of sessions. The figures are:

- crash-free sessions;
- the rate of Plux errors, of action errors, of function errors;
- the rate of custom events per session.

A regression beyond a threshold **pauses** the rollout (`PLX-3150`) by default, or rolls it back
(`PLX-3151`) when the rollout is configured to. Pause comes first by default. Either way the
organisation is notified ([notifications guide](notifications.md)). A sample that is too small
decides nothing, so a rollout with little traffic is not judged on noise.

The runtime reports the signals the gate reads (`session_start`, `session_end`, `fatal_error` and
uncaught Flutter errors). The gate uses no native crash SDK (maintainer decision Q13).

### Operating a rollout

- Create with `RolloutService.CreateRollout`; read with `GetRollout` and `ListRollouts`.
- Pause and resume with `PauseRollout` and `ResumeRollout`; stop with `AbortRollout`.
- Every state transition is audited, and a health decision names the figures it used.

## 4. Flags

A flag belongs to an environment and has a type: `bool`, `number`, `string` or `json` (with a
JSON Schema).

- The **default** lives in the app document, so a device has a value before it holds a control
  document.
- The **values** are per segment, ordered: the first segment a device belongs to wins. They reach
  devices through the control document of each channel, with no release.
- The app reads a flag as `flags.<key>` in PXL. `plux codegen` generates typed accessors for the
  host.
- A flag's type never changes (`PLX-8203`). To change the type, delete the flag and create it
  again.

Administer flags with `FlagService` (`CreateFlag`, `GetFlag`, `ListFlags`, `UpdateFlag`,
`DeleteFlag`).

## 5. Experiments

### Setting one up

An experiment is written as an experiment document in the project, so the compiler builds its
variants into the release. The document names:

- the **scope**: a page; a component subtree (a `Variant` node); a prop value (`experiment.<key>.<prop>`
  in PXL); a flow (a route mapping); or a plugin version (a channel split);
- the **control and variants**, with weights that total 100 (`PLX-1703` otherwise), and a
  `Variant` without a control is `PLX-1702`;
- the **allocation**, targeting and schedule;
- the **primary** metric and the **guardrail** metrics;
- the **minimum sample size** per variant.

Creating an experiment only registers it. Starting it writes its allocation into the control
documents of the environment; stopping it removes the allocation. `ExperimentService` has
`CreateExperiment`, `StartExperiment`, `StopExperiment`, `GetExperiment`, `ListExperiments` and
`DeleteExperiment`.

### Assignment and exposure

- A device assigns itself: `variant = weightedPick(sha256(experimentSalt || subject))`. The
  assignment works offline and is sticky.
- An **exposure** is recorded only when the variant's node is built on screen, by a renderer hook,
  not when the device is assigned. A user who was assigned but never shown the variant does not
  count as exposed.

### Layers and holdouts

Experiments that share a layer are mutually exclusive: each holds a range of the layer's 10,000
buckets while it runs. A start that finds no free range is refused (`PLX-8200`). A **holdout**
keeps a fixed share of the environment's subjects out of every experiment, so you can measure
the effect of all experiments together. Set it with `ExperimentService.SetHoldout`.

### Reading the results

`ExperimentService.GetResults` returns, for each variant against the control, the estimated
difference and whether it is significant.

- **Minimum sample.** A result is refused until every variant has the experiment's minimum sample
  size (`PLX-8201`). Do not read an estimate before then.
- **Sample ratio check.** The variants should receive their allocated shares. A deviation from the
  allocation is flagged (`PLX-8202`, a chi-squared test at p below 0.001). A flagged experiment has
  a problem with assignment or logging, so its effects are not trustworthy until the cause is found.
- **Continuous monitoring.** The confidence sequences stay valid when you look at the result at
  any time, so you need not fix the sample size in advance. A result that excludes zero is
  significant at the stated error rate; one that does not is not a proof of no effect.
- **Variance reduction.** The estimate uses the 14 days before each subject's first exposure as a
  covariate (CUPED) where it exists, which narrows the interval without biasing it.
- **Guardrails.** A guardrail that worsens matters even when the primary metric improves.

The statistics are in-house (`backend/internal/experiment/stats`), validated on simulated A/A and
A/B data with known effects, and the simulations check that the type I error stays at or below
the error rate under repeated peeking.

## 6. Promotion

When a variant wins, promote it: `ExperimentService.PromoteExperiment` names the winner. The
compiler replaces each `Variant` by the winner's subtree and inlines the `experiment.<key>`
bindings, so the promoted document produces the same bytes as one written with the winner
(`CMP-002`). The draft is replaced and a publish job runs through the normal publish path, with
every gate that path has. Nothing is bypassed.

Promotion refuses with `PLX-2300` when the experiment key is unknown, `PLX-2301` when the chosen
winner is not one of its variants, and `PLX-2302` when the promoted page would exceed the node
budget.

## 7. Approval for sensitive experiments

An experiment on a secure page or a money flow asks the approval engine before it starts
(`ABT-007`, [governance guide](governance.md)). Experiments that touch neither start without one.

## 8. Errors

| Code | Meaning |
|---|---|
| `PLX-3150` | Health gate paused the rollout. |
| `PLX-3151` | Health gate rolled the rollout back. |
| `PLX-3152` | Invalid targeting rule or segment. |
| `PLX-3153` | Another rollout of the channel is in progress. |
| `PLX-1701` | Unknown flag type. |
| `PLX-1702` | A variant has no control. |
| `PLX-1703` | Variant weights do not total 100. |
| `PLX-8200` | Experiment overlaps another in its layer, or no free range. |
| `PLX-8201` | Not enough data for a result. |
| `PLX-8202` | Sample ratio mismatch. |
| `PLX-8203` | A flag's type cannot change. |

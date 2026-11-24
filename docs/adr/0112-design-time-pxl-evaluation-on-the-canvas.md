<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0112. The canvas evaluates design-time expressions with the PXL engine, in the background

- **Status:** Accepted (maintainer, 2026-11-24)
- **Date:** 2026-11-24
- **Requirements:** `STU-005`, `DAT-080`, `SCH-024`

## Context and problem

The plugin canvas draws a widget bound to an expression with its design-time value: a page parameter's mock, a
state's default, a data source's mock under the chosen state (`DAT-080`, `SCH-024`). It read only what TypeScript
patterns could: plain paths, literals and simple comparisons. Everything else was drawn unevaluated, unlike on the
device (`STU-005`), as the maintainer found while building a real app (build log rows 117, 123, 138, 156, 237):

- `40.0 * device.textScale` in a size left the widget without that size;
- `format.number(item)` in a message argument drew `{n} km`;
- `string(device.platform)` drew nothing, and steps computed in PXL drew a placeholder `Stepper`.

Studio already ships the PXL engine: the validator's WebAssembly module checks every page and compiles each of its
expressions in its own scope. The maintainer decided the canvas uses it, "but not at a cost of performance or anything
else".

## Decision drivers

- The canvas draws what the device draws; PXL has one implementation, so the canvas cannot disagree with the runtime.
- No cost to the canvas's frame time or first load (spec §1.4, `STU-120`); nothing waits for the engine.
- No new trust: design-time values never leave the browser, and evaluation is bounded by the PXL limits.

## Considered options

1. Extend the TypeScript patterns case by case.
2. A second PXL interpreter in TypeScript.
3. The PXL engine in the validator's worker, asked in the background.

## Decision

Chosen option: **3**.

- **Where.** `compiler.DesignEvaluator` evaluates a page's expressions exactly as the page's check compiled them, each
  in its own scope (list items, guarded branches), with the registry's PXL limits. The validator module exposes it as
  `pluxPxlEval`; the validator worker answers `eval` requests beside validation.
- **Values.** The canvas sends each page as drawn, unsaved edits included, and the design-time values of its roots:
  parameters, page, plugin and app state, the data sources under their mock states, the preview's device, and a list
  item's `item` and `index` over its page's values. A root or field the canvas does not give, or one of another shape,
  reads as its type's empty value (`pxl.DesignFromJSON`), never an error; only the canvas reads values this way. `now`
  is the time the canvas opened. `format.*` and `t()` use the previewed locale's data and messages.
- **When.** The canvas's own readers stay the fast path. A pass over the screens reads what the engine answered
  before and leaves the rest as it is; afterwards the misses go to the worker, one batch per page, and each page's
  answers redraw only its screens. Answers are kept by page version, values and expression, so an unchanged screen
  asks nothing again; a change of locale or messages drops them. An expression that does not evaluate is drawn as
  before and not asked again.
- **What is not asked.** A component's own expressions (`props`, `component`), compiled with the component, and an
  action's (`event`, `steps`), which no widget draws; and an expression that reads `item` outside a copy of its list's
  template. Texts whose arguments read `item` are localised after the lists are drawn.

## Consequences

- **Positive:** bound sizes, formatted and translated arguments, device values, computed conditions and lists are drawn
  as the device draws them, from the same engine; first load and frame time are unchanged (measured on a 40-screen
  plugin: the loading state ended at 4.36–4.44 s with evaluation, 4.43–4.74 s without).
- **Negative:** a screen first draws with the unevaluated values and takes the evaluated ones a moment later; a large
  plugin's pages fill in one by one while the worker evaluates them. Expressions inside components are not evaluated
  yet.
- **Follow-up:** evaluate a component's own expressions with its instance's props.

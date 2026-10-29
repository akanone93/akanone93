<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0098. Library: templates and components authored in Studio

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9)
- **Date:** 2026-10-24
- **Requirements:** `STU-080` to `STU-086` (templates, visibility, dependencies, insertion, components, component versions, the library browser)

## Context and problem

The specification asks for two kinds of reusable content: **templates**, which are copied into a
plugin and then independent (`STU-080`, `STU-083`), and **components**, which are linked, versioned
and adopted by each app when it chooses (`STU-084`, `STU-085`). Both have visibility levels
(`STU-081`), and a library browser lists them (`STU-086`). The server has no resource for authoring
either: today there is no way to create, revise, version or share them, and no permission says who
may change what an organisation shares. Decisions: F8 in
[the functional decisions](../plans/p11-studio-functional-decisions.md); contract #7 and §6.2 of
[the P11 plan](../plans/p11.md).

## Decision drivers

- A shared component changes other people's apps, so changing it must be deliberate, visible and
  reversible (security and correctness before convenience).
- Public content reaches the whole installation, so it is scanned before it is shown (`STU-081`).
- Who may change shared content is enforced by the server, not hidden by Studio (`SEC-102`, M-7).
- Inserted templates must not silently change when the template does (`STU-083`).

## Considered options

1. First-class server resources with versions, visibility and per-app adoption, authored in Studio.
2. Templates and components as ordinary plugins that others copy or import.
3. Only file export and import, with no shared library.

## Decision

Chosen option: **1**.

- **Templates are copies, never links.** Inserting copies the nodes with new IDs, asks for the
  exposed parameters, and maps dependencies (tokens, assets, components, state, translation keys)
  with the same report as a paste ([ADR-0095](0095-studio-editing-model.md)). Afterwards the
  content is independent; a template change never reaches a plugin it was inserted into.
- **Components are linked and versioned.** An instance stores the component, a version and its
  overrides (prop values and slot fills); everything else comes from the component. Editing makes a
  new draft version; before it is saved, Studio lists the instances that would change across all
  apps that can see the component.
- **Adoption is per app and explicit.** An app adopts a new version with a diff (`STU-085`).
  Overrides stay where the prop still exists; overrides of removed props are flagged. **Detach**
  turns an instance into plain nodes with new IDs, as one undo step.
- **Visibility** is `private`, `organization` or `public` for both kinds. Changing to `public`
  runs the automated scan (no secrets, no private URLs), and moderation is optional.
- **Authoring resource.** One service creates, revises, versions and archives templates and
  components, sets visibility, runs the public scan, exports and imports files, and records the
  version an app adopts (contract #7). Thumbnails are produced in the browser and uploaded.
- **Permissions (server first, §6.2 of the plan).** `library.manage` lets a holder edit
  organisation-wide templates and components that are not their own; `library.publish` lets a
  holder publish a component version to the organisation's locked library (`STU-112`) or make
  content `public`. Both are added to `permissions.json` with area, feature, action and risk, given
  to roles in `roles.json`, checked in the handlers (deny by default) and covered by the
  authorisation-matrix test. The author edits their own content without either key. Content of a
  departing author passes to the organisation's administrators.
- **Export and import** are files in the `.plux` family (contract #24), schema-validated on import
  and treated as untrusted input.

## Consequences

- **Positive:** the library has one authoritative store; shared changes are previewed and adopted
  deliberately; the permissions are enforced on the server.
- **Negative:** a new resource and service with its tests; the public scan is a security control
  that needs maintenance; old component versions must be retained while any app adopts them.
- **Follow-up:** the two keys in the catalogue generated for TypeScript; error codes; usage counts
  for the library browser come from the adoption records.

## Options in detail

### Option 2 — plugins as library items

No new server resource, but plugins have no parameters, no visibility levels, no per-app version
adoption and no instance model; each of these would be bolted on. Rejected.

### Option 3 — files only

Safe and simple, yet nothing is shared inside an organisation and `STU-081` and `STU-085` cannot be
met. Rejected, but export and import stay as features.

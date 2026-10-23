<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0086. Restriction policies, effective permissions and explain

- **Status:** Accepted
- **Date:** 2026-10-21
- **Requirements:** `GOV-002`, `GOV-003`, `GOV-006`, `GOV-007`, `GOV-024`, `GOV-032`, `SEC-102`, `SEC-103`, `LIM-001`, `QA-008`

## Context and problem

ADR-0073 gives every person their rights as the union of their grants `(principal, role, scope)`.
Organisations also need to take rights away without editing roles: turn off publishing, one
feature, or all editing for a team or a person, and the Owner the same for the whole
organisation. Plux Studio (P11) must know which controls to hide or disable, and the server must
enforce the same answer. People and auditors must be able to ask why someone can or cannot do
something, and what a change would do before it is saved. The question: how are restrictions
modelled, evaluated, enforced in the database, cached and explained?

## Decision drivers

- Deny by default and deny wins (`SEC-080`, `SEC-102`); no combination of grants overrides a restriction.
- No lockout: the organisation can always be administered by its last Owner.
- No escalation: nobody restricts, or grants, what they do not hold.
- Handlers unchanged (L-1): `authorize` stays the one call; only the resolver changes.
- Access control is a security capability, so it is open-source core (`GOV-032`).
- Cheap on the hot path: one resolution per (person, organisation, app, environment), cached and invalidated on change.

## Considered options

1. Restrictions as stored deny rules over the catalogue, subtracted from the grants by the resolver; app-wide restrictions also in row-level security.
2. Negative permissions inside roles (a role carries allows and denies).
3. Attribute-based policies with a policy language (rejected for authorisation in ADR-0073).

## Decision

Chosen option: **1**, because it adds one subtraction to the resolver ADR-0073 already has, keeps
roles purely additive, and makes every denial nameable.

- **Catalogue (`GOV-002`).** `backend/internal/auth/permissions.json` stays the one source. Each
  entry gains `area`, `feature` and `action` fields and keeps its `scope` field (organisation,
  app or environment). Existing permission names are **not** renamed: stored grants, custom roles
  and generated code keep working; `plugin.edit` is area `plugin`, feature `plugin`, action
  `edit`. The generated documentation groups the table by area and feature. A feature's
  permissions are added in the phase that builds the feature (Studio's in P11), so every
  permission guards at least one call.
- **Data model.** Table `restriction_policies`: `id`, `organization_id`, `subject_kind`
  (`organization`, `team`, `user`), `subject_id` (null for `organization`), `patterns` (permission
  names or wildcards `area.*` and `area.feature.*`, validated against the catalogue at write
  time), optional `app_id`, optional `environment` (requires `app_id`), `reason`, `created_by`,
  `created_at`. Rows are immutable: a change is a delete and a create, both audited. Each
  organisation has a `permissions_version` counter, incremented in the same transaction as any
  change to grants, roles, team membership or restrictions.
- **Evaluation order.** For (person, organisation, app, environment): (1) collect the grants
  whose scope covers the app and environment (ADR-0073); (2) expand their roles into a permission
  set; (3) collect the restrictions whose subject is the organisation, a team the person belongs
  to, or the person, and whose app and environment scope covers the request; (4) remove every
  permission a restriction pattern matches; (5) re-add the **owner floor**: an Owner always keeps
  `members.manage`, `roles.manage`, `restrictions.manage` and `organization.manage`, so no
  restriction can lock the organisation out; (6) anything not in the result is denied. The result
  carries the organisation's `permissions_version`.
- **Who may restrict.** `restrictions.manage` (new; Owner and Admin, step-up `SEC-103`) for team
  and person restrictions; an organisation-wide restriction also needs `organization.manage`
  (Owner only). A caller may only restrict permissions they hold in the scope they name, and may
  not restrict themselves in a scope wider than their own grants. A change that would leave no
  Owner able to manage members, roles and restrictions is refused (`PLX-8105`) and audited, in
  addition to the resolver's owner floor.
- **Break-glass and freezes.** Break-glass (`GOV-024`) does not override a restriction: a
  restriction is a standing ceiling on people, break-glass an emergency path through approvals.
  A change freeze (`GOV-025`) is a time window on releases; both apply independently.
- **Row-level security (second barrier, `SEC-102`).** The database does not know which action a
  query serves, so per-action denial stays in the service layer. A restriction that removes every
  permission (`*`) for a person on an app is also enforced in PostgreSQL: the app-scoped tables
  get a restrictive policy `NOT plux_app_restricted(app_id)`, a `STABLE` function reading
  `restriction_policies` for `current_setting('plux.user_id')` and the person's teams. A person
  barred from an app can then neither read nor write its rows, even through a handler bug.
- **Effective permissions and change notices (`GOV-007`).** `GetEffectivePermissions` returns the
  compact set for (organisation, app, environment) with its version. `WatchPermissions` is a
  server-streaming call that sends the new version whenever the organisation's
  `permissions_version` changes; clients refetch. Nothing polls.
- **Explain and simulate.** `ExplainAccess` takes a person, a permission and a scope, and returns
  the decision with every grant that would allow it and every restriction that removes it (by
  ID), or `no grant`. With a proposed grant or restriction it evaluates the same resolver on the
  proposed state in memory and saves nothing. Anyone may explain their own access; another
  person's needs `members.manage`. Both calls have their own rate limit.
- **Caching and invalidation.** Each API replica caches resolved sets keyed by (person,
  organisation, app, environment, `permissions_version`). A change publishes the organisation's
  new version on a Valkey channel; replicas drop that organisation's entries. A bounded entry
  lifetime from the limits registry covers a missed message; when the subscription is down the
  cache is bypassed (fail closed to fresh resolution, never to stale grants).
- **Limits (`LIM-001`).** `auth.restrictionsPerOrg`, `auth.restrictionsPerSubject`,
  `auth.restrictionPatterns` and `auth.permissionCacheSeconds`, from the registry.
- **Errors.** `PLX-8104` restriction pattern invalid, `PLX-8105` change would lock out the last
  Owner, `PLX-8106` caller cannot restrict a permission or scope they do not hold, `PLX-8107`
  restriction limit reached. A denied call keeps `PLX-8030`; its detail names the restriction.
- **Audit.** Every grant, role, team-membership and restriction change, and every refused
  lockout or escalation attempt, is an audit entry.
- **Edition.** Core, not `ee/`: restrictions only remove rights and are access control
  (`GOV-032`). Custom roles stay enterprise (ADR-0073): they add composition, not protection.

## Consequences

- **Positive:** one subtraction in the existing resolver; deny is total and explainable; the
  lockout cannot happen through restrictions; Studio gets one set and one change notice to drive
  its controls; app-wide bans hold in the database.
- **Negative:** the authorisation matrix (`QA-008`) gains restriction cases; every change to
  grants or teams must bump the version, or caches go stale until their lifetime ends; a new
  restrictive RLS policy on app-scoped tables costs a function call per row check.
- **Follow-up:** P9 slice E1e implements it; Studio's restriction screens and explain view are
  P11; Studio's own permissions join the catalogue in P11 with their features.

## Options in detail

### Option 1 — restriction rules subtracted by the resolver

Described above. Roles stay additive and reviewable; denial has one place and one name.

### Option 2 — negative permissions in roles

Mixes allow and deny in one object, so a role's meaning depends on the other roles a person
holds; restricting one person means a new role per person. Rejected.

### Option 3 — attribute-based policies

Rejected for authorisation in ADR-0073; restrictions do not need attributes beyond subject, app
and environment.

# 0073. Roles, the permission catalogue, custom roles and scoping

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `GOV-001`, `GOV-002`, `GOV-003`, `GOV-021`, `GOV-032`, `SEC-103`, `QA-008`

## Context and problem

Today the server has five roles (`owner`, `admin`, `developer`, `viewer`, `translator`) and 22
permissions in `backend/internal/auth/rbac.go`, granted organisation-wide. A regulated
organisation needs more: a release manager who may publish but not edit code, an auditor who
reads everything including the audit log, a compliance officer, teams that approval policies can
refer to, and grants limited to one app or one environment (`GOV-002`, `GOV-003`). Customers
also need roles of their own. Upgrading must not change anyone's effective access, and every
handler must keep calling `authorize` (layering rule L-1). The question: how are permissions
defined, grouped into roles, scoped and extended?

## Decision drivers

- Least privilege and deny by default (`SEC-080`); a role can never grant more than its creator holds.
- One source of truth for the permission list, usable from Go, Dart and TypeScript, with generated documentation.
- Handlers unchanged: the resolver changes, not the call sites.
- Security capabilities are never edition-gated (`GOV-032`); only custom roles are enterprise.
- A testable authorisation matrix (`QA-008`).

## Considered options

1. A generated permission catalogue, ten predefined roles, grants as `(principal, role, scope)`, custom roles in the enterprise edition.
2. Keep the fixed five roles and add more fixed roles per request.
3. Attribute-based access control with a policy language evaluated per request.

## Decision

Chosen option: **1**, because it extends the existing model without touching call sites and
keeps the matrix finite and testable.

- **Catalogue.** `auth/permissions.json` is the single source. It lists every permission with
  description, risk and whether it needs step-up (ADR-0075). Go, Dart and TypeScript code and the
  documentation table are generated from it with `make gen`. The new permissions are those of
  plan §2.4 (`roles.manage`, `teams.manage`, `approvals.decide`, `rollout.manage`,
  `killswitch.manage`, `reconstruction.view`, `audit.export`, `license.manage`, and the rest).
- **Predefined roles (core).** Owner, Admin, Release Manager, Developer, Designer, Translator,
  Reviewer, Compliance Officer, Auditor (read-only including the audit log, plus
  `reconstruction.view` and `audit.export`) and Viewer. Migrations map the five existing roles
  one-to-one; a test proves nobody gains or loses access on upgrade.
- **Teams (core).** Users belong to teams. Approval policies refer to team membership
  (`GOV-021`, distinct teams); SCIM groups map to teams (ADR-0074).
- **Scoping (`GOV-003`).** A grant is `(principal, role, scope)`; scope is organisation, app, or
  app plus environment. Scoped permissions take the form `<permission>:<environment>`, for
  example `release.publish:production`, and resolve through the grant's scope. Every handler keeps
  calling `authorize`; only the resolver changes.
- **Custom roles (enterprise).** Composed from the catalogue. A custom role can never hold a
  permission its creator does not hold. Changing a role needs step-up (`SEC-103`) and, when a
  policy says so, approval (ADR-0076).
- **Errors.** `PLX-8100` role unknown, `8101` permission not in the catalogue, `8102` scope
  invalid, `8103` the last owner cannot be removed.
- The authorisation matrix test (`QA-008`) is regenerated for roles x permissions x scopes.

## Consequences

- **Positive:** one reviewed list of permissions; scoped grants without handler changes; custom
  roles cannot escalate; the upgrade is provably neutral.
- **Negative:** the matrix grows (roles x permissions x scopes); the catalogue becomes a contract
  that generated code in three languages follows; resolver changes carry security risk and need
  the 85% coverage floor for `auth`.
- **Follow-up:** migrations 0024 to 0027 (E1); Studio surfaces in P11; per-tenant roles in P15.

## Options in detail

### Option 1 — catalogue, predefined roles, scoped grants

Keeps the current RBAC shape. Costs a migration and a generator. Custom roles being enterprise
does not weaken security: the predefined ten cover least privilege for the common cases.

### Option 2 — more fixed roles

Cheapest, but each customer's need produces another role and an upgrade; no scoping, so
`GOV-003` stays unmet. Rejected.

### Option 3 — attribute-based policies

Most expressive, but hard to review, hard to test exhaustively, and a new language in the
authorisation path. The approval engine (ADR-0076) already covers the conditional cases that
need attributes (risk, environment). Rejected for authorisation.

## Revision (2026-10-24, P11 plan: Studio commands and permissions)

Every Studio command maps to the permission of the RPC it calls, declared in the command registry and typed against the catalogue generated for TypeScript, so an unknown name does not compile. When a command's action is not covered by an existing permission, the permission is added in the same change, server first: an entry in `permissions.json` (area, feature, action, risk), role assignments in `roles.json`, a check in the RPC handler (deny by default, `SEC-102`) and coverage in the authorisation-matrix test for every role (`QA-008`). The addition is listed in the change for the maintainer's approval (AGENTS §7); only then does the Studio command use it.

A switch that only shapes the interface, such as hiding the JSON tab from a role that may still edit through Properties, is a profile or organisation setting, never a permission, and is documented as not being a security control.

New keys: `library.manage` and `library.publish` for shared templates and components ([ADR-0098](0098-library-templates-and-components.md)). The plan's §6.2 holds the rule in full.

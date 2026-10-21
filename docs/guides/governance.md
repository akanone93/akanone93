<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Governance Guide

Who may do what in a Plux organisation, how a sensitive change gets a second person's
agreement, how an emergency is handled, and how an administrator can see and limit access.
The requirements are `GOV-002` to `GOV-007`, `GOV-020` to `GOV-026` and
[ADR-0073](../adr/0073-roles-permissions-and-scoping.md),
[ADR-0076](../adr/0076-approval-engine.md) and
[ADR-0086](../adr/0086-restriction-policies-and-effective-permissions.md).

Edition note: roles, scoped grants, restriction policies, approvals with a single stage, and
break-glass are in the core. Custom roles, multi-stage and distinct-team policies, change
freezes and compliance reports are enterprise features (`GOV-032`: security capabilities are
never gated by edition, so access control itself is always in the core).

## 1. The idea in one page

Every call to the Plux Server names the permission it needs. A person holds permissions
through **grants**: a role, bound to a person or a team, at a scope. Three rules shape the
rest of this guide:

1. **Deny by default.** A permission that no grant gives is refused (`SEC-102`).
2. **Restrictions win.** A restriction removes permissions from a person, a team or the whole
   organisation whatever their grants say (section 6).
3. **Sensitive changes need a second person.** A change that matches an approval policy waits
   for approvers. The approval is about one exact artifact, so a later change does not inherit
   it (section 5).

## 2. Roles

Ten roles are predefined. Their permission lists are in
[the permissions reference](../reference/permissions.md), which is generated from the catalogue.

| Role | What it is for |
|---|---|
| `owner` | Everything, including members, roles, identity providers, keys and the licence. |
| `admin` | Everything except managing members and keys. |
| `release-manager` | Publishes, promotes and rolls back releases; runs rollouts, flags and experiments; uses the kill switch and break-glass. Does not edit code. |
| `developer` | Edits plugins, deploys functions outside production, publishes. |
| `designer` | Edits plugins, pages and translations; does not publish. |
| `translator` | Reads and edits translations and nothing else. |
| `reviewer` | Reads what an approver needs; approves or rejects changes. |
| `compliance-officer` | Reads the audit log and reconstructions. |
| `auditor` | Read-only, including the audit log, reconstructions and audit export. |
| `viewer` | Reads. |

A role never grants more than its creator holds, so an administrator cannot hand out an
authority they do not have (ADR-0073).

## 3. Permissions

The catalogue is the single source (`backend/internal/auth/permissions.json`). Permissions are
grouped by area and feature, each with an action; existing names such as `plugin.edit` keep
their form (ADR-0086). For each one the reference shows:

- **Risk**: the damage a misused permission can do.
- **Step-up**: whether the permission works only in a session that has presented a second
  factor recently (section 4 of the [sessions guide](sessions-and-step-up.md)).
- **Scope**: the narrowest scope it can be granted at. `organization` permissions apply to the
  whole organisation; `app` permissions can also be granted on one app; `environment`
  permissions can also be granted on one environment of an app, written
  `<permission>:<environment>`, for example `release.publish:production` (`GOV-003`).

Permission names are stable. A new feature adds its permissions in the phase that builds it,
so every permission guards at least one RPC (see the authorisation matrix in the engineering
handbook).

## 4. Grants and scopes

A grant is `(person or team, role, scope)`. The scope is the organisation, one app, or one
environment of an app. A person's effective permissions in a scope are the union of the
permissions of the roles they hold there, minus any restriction (section 6).

Administer grants with the CLI (see [the CLI reference](../reference/cli.md)):

- `plux grants list [--user id] [--team id] [--app id]`
- `plux grants create --role key (--user id | --team id) [--scope organization|app|environment] [--app id] [--environment id]`
- `plux grants update --role key <id>` and `plux grants delete <id>`

Teams group people. Approval policies can require approvers from distinct teams, and SCIM
groups map to teams ([SSO and SCIM guide](sso-and-scim.md)).

## 5. Custom roles

Custom roles are an enterprise feature. A custom role is a named set of catalogue permissions,
with the same rules as a predefined role:

- `plux roles create <key> [--name text] [--description text] [--permissions a,b]`
- `plux roles update <key>`, `plux roles delete <key>`, `plux roles show <key>`, `plux roles list`

A custom role can never hold a permission its creator does not hold in the scope it is granted
at.

## 6. Restriction policies

A restriction is a ceiling over grants (`GOV-006`, ADR-0086). It removes permissions from its
subject:

- **Subject**: the organisation, a team, or one person.
- **Patterns**: catalogue permissions or the wildcards `*`, `<area>.*` and `<area>.<feature>.*`.
- **Scope**: optionally one app, and one environment of that app.
- **Reason**: free text, recorded in the audit log.

Create and list them with `plux restrictions create --patterns a,b [--subject-kind organization|team|user] [--subject id] [--app id] [--environment key] [--reason text]`,
`plux restrictions list`, and `plux restrictions delete <id>`. A restriction is immutable: a
change is a delete and a create, both audited.

Rules that keep restrictions safe:

- **Who may restrict.** Creating a restriction needs `restrictions.manage` with a fresh step-up.
  An organisation-wide restriction also needs `organization.manage`, which only an Owner holds.
  Nobody restricts a permission they do not hold in the scope they name.
- **Owner floor.** An Owner always keeps `members.manage`, `roles.manage`,
  `restrictions.manage` and `organization.manage`, so no restriction can lock the organisation
  out.
- **No lockout.** A change that would leave no Owner able to manage members, roles and
  restrictions is refused with `PLX-8105` and audited.
- **Break-glass and freezes are not overridden.** A restriction is a standing limit on people;
  break-glass is an emergency path through approvals, and freezes are time windows on releases.
  Each applies independently.

## 7. Effective permissions and explain

Two questions come up in every review: "what can this person do here?" and "why can't they do
this?"

- **Effective permissions.** `GovernanceService.GetEffectivePermissions` returns the set a
  person holds in a scope, with the organisation's `permissions_version` it was resolved at. A
  client refetches when the version moves; `WatchPermissions` announces the change.
- **Explain.** `plux access explain --permission name [--user id] [--app id] [--environment key]`
  says why a person can or cannot use a permission in a scope. It can also evaluate a proposed
  grant or restriction (`--grant-role`, `--restrict`) without saving it, so an administrator
  sees the effect before making it. The same answer is `GovernanceService.ExplainAccess`.

The evaluation order is fixed (ADR-0086): collect grants that cover the scope, expand their
roles, collect restrictions that apply to the person, remove every permission a restriction
matches, add back the owner floor, and deny anything not in the result.

## 8. Approvals and policies

An approval policy says which changes need agreement, and how many people must give it. Policies
are documents, versioned and audited like any other, and exported with the app's Git layout.

### What a policy matches and requires

- **Match**: resource type (release, function version, policy, security configuration, limit,
  capability set, experiment, accessibility waiver, golden baseline), environment, action, and
  risk attributes such as touching a secure page or a money flow. The compiler and the release
  service compute the risk attributes from the diff.
- **Require**: a number of approvers, roles, named approvers, distinct from the author, distinct
  teams, stages (sequential or parallel) and an expiry. Stages and distinct teams are enterprise.

A change that matches several policies needs all of them satisfied.

### Defaults per security profile

The first time an app is asked about, the defaults of its security profile are written as
ordinary policy documents that the organisation can tighten. A default an administrator deleted
is not written again.

- `maximum`: four-eyes on production publish, function deploy, device-function enablement,
  security-configuration change, accessibility waiver and golden approval.
- `standard`: one approver for production.
- `development`: none.

### Binding to the artifact

An approval names the artifact by its content hash: the release identifier and manifest hash, a
function version hash, or a policy document hash. A change to the content makes a new hash, so
earlier approvals stop counting and the change answers `PLX-8002` (`GOV-022`).

When no approval is on file, the change is refused with `PLX-8001` and a request is opened
(in its own transaction, so it survives the refusal). `PLX-8004` means the policy's expiry
passed. An approval is spent by the change it approves.

### Deciding

Approvers decide with `ApprovalService.DecideApproval`, which needs a fresh step-up. The
requester and the author of the change are never eligible (`PLX-8003`), nor is anyone the
policy's roles and named approvers leave out. Each decision is a record written to the audit
chain and signed later by the audit signing role, so an auditor can verify it offline.

The review bundle that an approver reads comes from `ApprovalService.GetApprovalReview`: the
JSON diff, validation and accessibility reports, golden-image differences, the install base, the
changelog and a risk summary.

Policies: `ApprovalService.ListPolicies`, `GetPolicy`, `PutPolicy`, `DeletePolicy`,
`ExportPolicies`. Requests: `ListApprovals`, `GetApproval`.

## 9. Break-glass

Break-glass lets a designated role act without waiting for approvals, for an emergency such as a
kill switch on a broken page, a rollback, or a disabled function (`GOV-024`).

- The role needs `breakglass.invoke`. The caller gives a written justification with the action
  (the `break_glass` and `justification` fields of the control and kill-switch calls). Without
  one the action is refused with `PLX-8005`.
- Approvers are alerted at once through the notification channels ([notifications guide](notifications.md)).
- A post-hoc review is opened. It is due within `governance.breakGlassReviewHours`, which
  defaults to 24. An overdue review escalates (`PLX-8006`).
- Review: `BreakGlassService.ListBreakGlass` lists the uses; `ReviewBreakGlass` records the
  post-hoc review.

Break-glass does not override a restriction (section 6).

## 10. Change freezes (enterprise)

A freeze is a calendar window in which production changes are refused, one-off or recurring,
in a time zone, per organisation and environment. A refused production change answers
`PLX-8010` and can only proceed through break-glass. Manage freezes with `FreezeService.PutFreeze`,
`ListFreezes` and `DeleteFreeze`.

## 11. Evidence

Every state change above writes an audit entry: grants, roles, restrictions, policies,
decisions, break-glass and freezes. Audit entries are never deleted (`SEC-140`), so the chain
stays verifiable. Compliance reports for a period (`GOV-026`, enterprise) list every production
change with its author, approvers, timestamps and content hashes. The [compliance index](../compliance/README.md)
lists where the evidence is.

## 12. Errors

| Code | Meaning |
|---|---|
| `PLX-8001` | Approval required; a request was opened. |
| `PLX-8002` | The approved artifact changed; earlier approvals no longer count. |
| `PLX-8003` | The approver is not eligible (requester, author, or not listed). |
| `PLX-8004` | The policy's approval has expired. |
| `PLX-8005` | Break-glass needs a justification. |
| `PLX-8006` | The break-glass review is overdue. |
| `PLX-8010` | A change freeze refuses the production change. |
| `PLX-8105` | The change would leave no Owner able to administer the organisation. |

See [the error reference](../reference/errors.md) for all codes.

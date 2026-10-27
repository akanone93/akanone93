// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the organisation administration area (STU-116). */
export function adminMessages(i18n: I18n) {
  return {
    navLabel: i18n._({
      id: "admin.navLabel",
      message: "Administration",
      comment: "Text in the organisation administration area.",
    }),
    navSettings: i18n._({
      id: "admin.navSettings",
      message: "Settings",
      comment: "Text in the organisation administration area.",
    }),
    navMembers: i18n._({
      id: "admin.navMembers",
      message: "Members",
      comment: "Text in the organisation administration area.",
    }),
    navTeams: i18n._({
      id: "admin.navTeams",
      message: "Teams",
      comment: "Text in the organisation administration area.",
    }),
    navRoles: i18n._({
      id: "admin.navRoles",
      message: "Roles",
      comment: "Text in the organisation administration area.",
    }),
    navRestrictions: i18n._({
      id: "admin.navRestrictions",
      message: "Restrictions",
      comment: "Text in the organisation administration area.",
    }),
    navAccess: i18n._({
      id: "admin.navAccess",
      message: "Explain access",
      comment: "Text in the organisation administration area.",
    }),
    navSso: i18n._({
      id: "admin.navSso",
      message: "SSO and SCIM",
      comment: "Text in the organisation administration area.",
    }),
    navTokens: i18n._({
      id: "admin.navTokens",
      message: "Tokens",
      comment: "Text in the organisation administration area.",
    }),
    navSessions: i18n._({
      id: "admin.navSessions",
      message: "Sessions",
      comment: "Text in the organisation administration area.",
    }),
    navAudit: i18n._({
      id: "admin.navAudit",
      message: "Audit log",
      comment: "Text in the organisation administration area.",
    }),
    navCompliance: i18n._({
      id: "admin.navCompliance",
      message: "Compliance",
      comment: "Text in the organisation administration area.",
    }),
    navRetention: i18n._({
      id: "admin.navRetention",
      message: "Retention",
      comment: "Text in the organisation administration area.",
    }),
    navLicense: i18n._({
      id: "admin.navLicense",
      message: "Licence",
      comment: "Text in the organisation administration area.",
    }),
    navLimits: i18n._({
      id: "admin.navLimits",
      message: "Limits",
      comment: "Text in the organisation administration area.",
    }),
    loading: i18n._({
      id: "admin.loading",
      message: "Loading",
      comment: "Text in the organisation administration area.",
    }),
    loadFailed: i18n._({
      id: "admin.loadFailed",
      message: "This could not be loaded.",
      comment: "Text in the organisation administration area.",
    }),
    saveFailed: i18n._({
      id: "admin.saveFailed",
      message: "The change was not saved.",
      comment: "Text in the organisation administration area.",
    }),
    saved: i18n._({ id: "admin.saved", message: "Saved.", comment: "Text in the organisation administration area." }),
    nothing: i18n._({
      id: "admin.nothing",
      message: "Nothing to show.",
      comment: "Text in the organisation administration area.",
    }),
    noPermission: i18n._({
      id: "admin.noPermission",
      message: "You lack the permission for this action.",
      comment: "Text in the organisation administration area.",
    }),
    dismiss: i18n._({
      id: "admin.dismiss",
      message: "Dismiss",
      comment: "Text in the organisation administration area.",
    }),
    apply: i18n._({ id: "admin.apply", message: "Apply", comment: "Text in the organisation administration area." }),
    save: i18n._({ id: "admin.save", message: "Save", comment: "Text in the organisation administration area." }),
    remove: i18n._({ id: "admin.remove", message: "Remove", comment: "Text in the organisation administration area." }),
    delete: i18n._({ id: "admin.delete", message: "Delete", comment: "Text in the organisation administration area." }),
    revoke: i18n._({ id: "admin.revoke", message: "Revoke", comment: "Text in the organisation administration area." }),
    yes: i18n._({ id: "admin.yes", message: "Yes", comment: "Text in the organisation administration area." }),
    no: i18n._({ id: "admin.no", message: "No", comment: "Text in the organisation administration area." }),
    colName: i18n._({ id: "admin.colName", message: "Name", comment: "Text in the organisation administration area." }),
    colKey: i18n._({ id: "admin.colKey", message: "Key", comment: "Text in the organisation administration area." }),
    colEmail: i18n._({
      id: "admin.colEmail",
      message: "Email",
      comment: "Text in the organisation administration area.",
    }),
    colRole: i18n._({ id: "admin.colRole", message: "Role", comment: "Text in the organisation administration area." }),
    colUser: i18n._({ id: "admin.colUser", message: "User", comment: "Text in the organisation administration area." }),
    colAdded: i18n._({
      id: "admin.colAdded",
      message: "Added",
      comment: "Text in the organisation administration area.",
    }),
    colCreated: i18n._({
      id: "admin.colCreated",
      message: "Created",
      comment: "Text in the organisation administration area.",
    }),
    colExpires: i18n._({
      id: "admin.colExpires",
      message: "Expires",
      comment: "Text in the organisation administration area.",
    }),
    colLastUsed: i18n._({
      id: "admin.colLastUsed",
      message: "Last used",
      comment: "Text in the organisation administration area.",
    }),
    colState: i18n._({
      id: "admin.colState",
      message: "State",
      comment: "Text in the organisation administration area.",
    }),
    stateActive: i18n._({
      id: "admin.stateActive",
      message: "Active",
      comment: "Text in the organisation administration area.",
    }),
    stateRevoked: i18n._({
      id: "admin.stateRevoked",
      message: "Revoked",
      comment: "Text in the organisation administration area.",
    }),
    never: i18n._({ id: "admin.never", message: "Never", comment: "Text in the organisation administration area." }),
    settingsTitle: i18n._({
      id: "admin.settingsTitle",
      message: "Organisation settings",
      comment: "Text in the organisation administration area.",
    }),
    settingsCaption: i18n._({
      id: "admin.settingsCaption",
      message: "Rename the organisation and open every administration page.",
      comment: "Text in the organisation administration area.",
    }),
    orgName: i18n._({
      id: "admin.orgName",
      message: "Organisation name",
      comment: "Text in the organisation administration area.",
    }),
    membersTitle: i18n._({
      id: "admin.membersTitle",
      message: "Members and invitations",
      comment: "Text in the organisation administration area.",
    }),
    membersCaption: i18n._({
      id: "admin.membersCaption",
      message: "People in this organisation and their roles.",
      comment: "Text in the organisation administration area.",
    }),
    inviteEmail: i18n._({
      id: "admin.inviteEmail",
      message: "Email address to invite",
      comment: "Text in the organisation administration area.",
    }),
    invite: i18n._({ id: "admin.invite", message: "Invite", comment: "Text in the organisation administration area." }),
    inviteLink: i18n._({
      id: "admin.inviteLink",
      message: "Invitation (shown once)",
      comment: "Text in the organisation administration area.",
    }),
    secretNote: i18n._({
      id: "admin.secretNote",
      message: "Copy it now. It is never shown again.",
      comment: "Text in the organisation administration area.",
    }),
    removeMember: i18n._({
      id: "admin.removeMember",
      message: "Remove member",
      comment: "Text in the organisation administration area.",
    }),
    removeMemberDesc: i18n._({
      id: "admin.removeMemberDesc",
      message: "This person loses access to the organisation.",
      comment: "Text in the organisation administration area.",
    }),
    memberTitle: i18n._({
      id: "admin.memberTitle",
      message: "Member",
      comment: "Text in the organisation administration area.",
    }),
    grantsHeading: i18n._({
      id: "admin.grantsHeading",
      message: "Grants",
      comment: "Text in the organisation administration area.",
    }),
    grantRole: i18n._({
      id: "admin.grantRole",
      message: "Role to grant",
      comment: "Text in the organisation administration area.",
    }),
    grantApp: i18n._({
      id: "admin.grantApp",
      message: "App ID (empty for the organisation)",
      comment: "Text in the organisation administration area.",
    }),
    grantScope: i18n._({
      id: "admin.grantScope",
      message: "Scope",
      comment: "Text in the organisation administration area.",
    }),
    addGrant: i18n._({
      id: "admin.addGrant",
      message: "Add grant",
      comment: "Text in the organisation administration area.",
    }),
    restrictionsHeading: i18n._({
      id: "admin.restrictionsHeading",
      message: "Restrictions",
      comment: "Text in the organisation administration area.",
    }),
    sessionsHeading: i18n._({
      id: "admin.sessionsHeading",
      message: "Sessions",
      comment: "Text in the organisation administration area.",
    }),
    explainLink: i18n._({
      id: "admin.explainLink",
      message: "Explain access",
      comment: "Text in the organisation administration area.",
    }),
    teamsTitle: i18n._({
      id: "admin.teamsTitle",
      message: "Teams",
      comment: "Text in the organisation administration area.",
    }),
    teamsCaption: i18n._({
      id: "admin.teamsCaption",
      message: "Groups of people that receive grants and restrictions together.",
      comment: "Text in the organisation administration area.",
    }),
    teamTitle: i18n._({
      id: "admin.teamTitle",
      message: "Team",
      comment: "Text in the organisation administration area.",
    }),
    createTeam: i18n._({
      id: "admin.createTeam",
      message: "Create team",
      comment: "Text in the organisation administration area.",
    }),
    teamMembers: i18n._({
      id: "admin.teamMembers",
      message: "Team members",
      comment: "Text in the organisation administration area.",
    }),
    rolesTitle: i18n._({
      id: "admin.rolesTitle",
      message: "Roles",
      comment: "Text in the organisation administration area.",
    }),
    rolesCaption: i18n._({
      id: "admin.rolesCaption",
      message: "Predefined roles and the custom roles of this organisation.",
      comment: "Text in the organisation administration area.",
    }),
    colPredefined: i18n._({
      id: "admin.colPredefined",
      message: "Predefined",
      comment: "Text in the organisation administration area.",
    }),
    newRole: i18n._({
      id: "admin.newRole",
      message: "New role",
      comment: "Text in the organisation administration area.",
    }),
    builderTitle: i18n._({
      id: "admin.builderTitle",
      message: "Role builder",
      comment: "Text in the organisation administration area.",
    }),
    builderCaption: i18n._({
      id: "admin.builderCaption",
      message: "Pick the permissions of the role. Areas hold features, features hold actions.",
      comment: "Text in the organisation administration area.",
    }),
    permissionsTree: i18n._({
      id: "admin.permissionsTree",
      message: "Permissions",
      comment: "Text in the organisation administration area.",
    }),
    roleDescription: i18n._({
      id: "admin.roleDescription",
      message: "Description",
      comment: "Text in the organisation administration area.",
    }),
    needsStepUp: i18n._({
      id: "admin.needsStepUp",
      message: "needs step-up",
      comment: "Text in the organisation administration area.",
    }),
    predefinedLocked: i18n._({
      id: "admin.predefinedLocked",
      message: "Predefined roles cannot be changed.",
      comment: "Text in the organisation administration area.",
    }),
    deleteRole: i18n._({
      id: "admin.deleteRole",
      message: "Delete role",
      comment: "Text in the organisation administration area.",
    }),
    deleteRoleDesc: i18n._({
      id: "admin.deleteRoleDesc",
      message: "Grants that use this role stop applying.",
      comment: "Text in the organisation administration area.",
    }),
    generalFeature: i18n._({
      id: "admin.generalFeature",
      message: "general",
      comment: "Text in the organisation administration area.",
    }),
    restrictionsTitle: i18n._({
      id: "admin.restrictionsTitle",
      message: "Restriction policies",
      comment: "Text in the organisation administration area.",
    }),
    restrictionsCaption: i18n._({
      id: "admin.restrictionsCaption",
      message: "Ceilings that remove permissions whatever the grants say. Deny always wins.",
      comment: "Text in the organisation administration area.",
    }),
    subjectKind: i18n._({
      id: "admin.subjectKind",
      message: "Applies to",
      comment: "Text in the organisation administration area.",
    }),
    kindOrg: i18n._({
      id: "admin.kindOrg",
      message: "Organisation",
      comment: "Text in the organisation administration area.",
    }),
    kindTeam: i18n._({
      id: "admin.kindTeam",
      message: "Team",
      comment: "Text in the organisation administration area.",
    }),
    kindUser: i18n._({
      id: "admin.kindUser",
      message: "Person",
      comment: "Text in the organisation administration area.",
    }),
    subjectId: i18n._({
      id: "admin.subjectId",
      message: "Team or person ID",
      comment: "Text in the organisation administration area.",
    }),
    patterns: i18n._({
      id: "admin.patterns",
      message: "Permission patterns (comma-separated)",
      comment: "Text in the organisation administration area.",
    }),
    reason: i18n._({ id: "admin.reason", message: "Reason", comment: "Text in the organisation administration area." }),
    addRestriction: i18n._({
      id: "admin.addRestriction",
      message: "Add restriction",
      comment: "Text in the organisation administration area.",
    }),
    lockoutNote: i18n._({
      id: "admin.lockoutNote",
      message: "The last owner can never lose the ability to manage members and policies.",
      comment: "Text in the organisation administration area.",
    }),
    colSubject: i18n._({
      id: "admin.colSubject",
      message: "Applies to",
      comment: "Text in the organisation administration area.",
    }),
    colPatterns: i18n._({
      id: "admin.colPatterns",
      message: "Patterns",
      comment: "Text in the organisation administration area.",
    }),
    accessTitle: i18n._({
      id: "admin.accessTitle",
      message: "Explain access",
      comment: "Text in the organisation administration area.",
    }),
    accessCaption: i18n._({
      id: "admin.accessCaption",
      message: "Why a person holds or lacks a permission, naming the grant or restriction.",
      comment: "Text in the organisation administration area.",
    }),
    accessUser: i18n._({
      id: "admin.accessUser",
      message: "User ID",
      comment: "Text in the organisation administration area.",
    }),
    accessPermission: i18n._({
      id: "admin.accessPermission",
      message: "Permission",
      comment: "Text in the organisation administration area.",
    }),
    accessApp: i18n._({
      id: "admin.accessApp",
      message: "App ID",
      comment: "Text in the organisation administration area.",
    }),
    accessEnv: i18n._({
      id: "admin.accessEnv",
      message: "Environment",
      comment: "Text in the organisation administration area.",
    }),
    explain: i18n._({
      id: "admin.explain",
      message: "Explain",
      comment: "Text in the organisation administration area.",
    }),
    allowed: i18n._({
      id: "admin.allowed",
      message: "Allowed",
      comment: "Text in the organisation administration area.",
    }),
    denied: i18n._({ id: "admin.denied", message: "Denied", comment: "Text in the organisation administration area." }),
    allowedBy: i18n._({
      id: "admin.allowedBy",
      message: "Granted by",
      comment: "Text in the organisation administration area.",
    }),
    removedBy: i18n._({
      id: "admin.removedBy",
      message: "Removed by restriction",
      comment: "Text in the organisation administration area.",
    }),
    ownerFloor: i18n._({
      id: "admin.ownerFloor",
      message: "The owner floor applies: the last owner keeps this permission.",
      comment: "Text in the organisation administration area.",
    }),
    ssoTitle: i18n._({
      id: "admin.ssoTitle",
      message: "SSO and SCIM",
      comment: "Text in the organisation administration area.",
    }),
    ssoCaption: i18n._({
      id: "admin.ssoCaption",
      message: "Identity providers of this organisation and the tokens that let a directory provision people.",
      comment: "Text in the organisation administration area.",
    }),
    providers: i18n._({
      id: "admin.providers",
      message: "Identity providers",
      comment: "Text in the organisation administration area.",
    }),
    colKind: i18n._({ id: "admin.colKind", message: "Kind", comment: "Text in the organisation administration area." }),
    colEnabled: i18n._({
      id: "admin.colEnabled",
      message: "Enabled",
      comment: "Text in the organisation administration area.",
    }),
    scimTokens: i18n._({
      id: "admin.scimTokens",
      message: "SCIM tokens",
      comment: "Text in the organisation administration area.",
    }),
    tokenName: i18n._({
      id: "admin.tokenName",
      message: "Token name",
      comment: "Text in the organisation administration area.",
    }),
    createScim: i18n._({
      id: "admin.createScim",
      message: "Create SCIM token",
      comment: "Text in the organisation administration area.",
    }),
    rotate: i18n._({ id: "admin.rotate", message: "Rotate", comment: "Text in the organisation administration area." }),
    secretHeading: i18n._({
      id: "admin.secretHeading",
      message: "Secret (shown once)",
      comment: "Text in the organisation administration area.",
    }),
    tokensTitle: i18n._({
      id: "admin.tokensTitle",
      message: "API and CI tokens",
      comment: "Text in the organisation administration area.",
    }),
    tokensCaption: i18n._({
      id: "admin.tokensCaption",
      message: "Tokens for scripts and pipelines, and the CI workloads trusted without a token.",
      comment: "Text in the organisation administration area.",
    }),
    tokenScopes: i18n._({
      id: "admin.tokenScopes",
      message: "Scopes (comma-separated)",
      comment: "Text in the organisation administration area.",
    }),
    tokenDays: i18n._({
      id: "admin.tokenDays",
      message: "Lifetime in days",
      comment: "Text in the organisation administration area.",
    }),
    createToken: i18n._({
      id: "admin.createToken",
      message: "Create token",
      comment: "Text in the organisation administration area.",
    }),
    colPrefix: i18n._({
      id: "admin.colPrefix",
      message: "Prefix",
      comment: "Text in the organisation administration area.",
    }),
    workloads: i18n._({
      id: "admin.workloads",
      message: "Workload identities",
      comment: "Text in the organisation administration area.",
    }),
    issuer: i18n._({ id: "admin.issuer", message: "Issuer", comment: "Text in the organisation administration area." }),
    audience: i18n._({
      id: "admin.audience",
      message: "Audience",
      comment: "Text in the organisation administration area.",
    }),
    subjectPattern: i18n._({
      id: "admin.subjectPattern",
      message: "Subject pattern",
      comment: "Text in the organisation administration area.",
    }),
    createWorkload: i18n._({
      id: "admin.createWorkload",
      message: "Add workload identity",
      comment: "Text in the organisation administration area.",
    }),
    sessionsTitle: i18n._({
      id: "admin.sessionsTitle",
      message: "Active sessions",
      comment: "Text in the organisation administration area.",
    }),
    sessionsCaption: i18n._({
      id: "admin.sessionsCaption",
      message: "Sign-ins of people in this organisation. Revoking signs the session out.",
      comment: "Text in the organisation administration area.",
    }),
    colClient: i18n._({
      id: "admin.colClient",
      message: "Client",
      comment: "Text in the organisation administration area.",
    }),
    colSeen: i18n._({
      id: "admin.colSeen",
      message: "Last seen",
      comment: "Text in the organisation administration area.",
    }),
    colNetwork: i18n._({
      id: "admin.colNetwork",
      message: "Network",
      comment: "Text in the organisation administration area.",
    }),
    auditTitle: i18n._({
      id: "admin.auditTitle",
      message: "Audit log",
      comment: "Text in the organisation administration area.",
    }),
    auditCaption: i18n._({
      id: "admin.auditCaption",
      message: "Every change of this organisation, searchable and exportable.",
      comment: "Text in the organisation administration area.",
    }),
    filterFrom: i18n._({
      id: "admin.filterFrom",
      message: "From (date)",
      comment: "Text in the organisation administration area.",
    }),
    filterTo: i18n._({
      id: "admin.filterTo",
      message: "To (date)",
      comment: "Text in the organisation administration area.",
    }),
    filterActor: i18n._({
      id: "admin.filterActor",
      message: "Actor",
      comment: "Text in the organisation administration area.",
    }),
    filterAction: i18n._({
      id: "admin.filterAction",
      message: "Action",
      comment: "Text in the organisation administration area.",
    }),
    filterResource: i18n._({
      id: "admin.filterResource",
      message: "Resource",
      comment: "Text in the organisation administration area.",
    }),
    colTime: i18n._({ id: "admin.colTime", message: "Time", comment: "Text in the organisation administration area." }),
    colActor: i18n._({
      id: "admin.colActor",
      message: "Actor",
      comment: "Text in the organisation administration area.",
    }),
    colAction: i18n._({
      id: "admin.colAction",
      message: "Action",
      comment: "Text in the organisation administration area.",
    }),
    colResource: i18n._({
      id: "admin.colResource",
      message: "Resource",
      comment: "Text in the organisation administration area.",
    }),
    exportCsv: i18n._({
      id: "admin.exportCsv",
      message: "Export CSV",
      comment: "Text in the organisation administration area.",
    }),
    destinations: i18n._({
      id: "admin.destinations",
      message: "Export destinations",
      comment: "Text in the organisation administration area.",
    }),
    moreEntries: i18n._({
      id: "admin.moreEntries",
      message: "Load more",
      comment: "Text in the organisation administration area.",
    }),
    complianceTitle: i18n._({
      id: "admin.complianceTitle",
      message: "Compliance reports",
      comment: "Text in the organisation administration area.",
    }),
    complianceCaption: i18n._({
      id: "admin.complianceCaption",
      message: "Signed reports of the changes in a period.",
      comment: "Text in the organisation administration area.",
    }),
    periodFrom: i18n._({
      id: "admin.periodFrom",
      message: "Period from",
      comment: "Text in the organisation administration area.",
    }),
    periodTo: i18n._({
      id: "admin.periodTo",
      message: "Period to",
      comment: "Text in the organisation administration area.",
    }),
    createReport: i18n._({
      id: "admin.createReport",
      message: "Create report",
      comment: "Text in the organisation administration area.",
    }),
    colPeriod: i18n._({
      id: "admin.colPeriod",
      message: "Period",
      comment: "Text in the organisation administration area.",
    }),
    colChanges: i18n._({
      id: "admin.colChanges",
      message: "Changes",
      comment: "Text in the organisation administration area.",
    }),
    colSigned: i18n._({
      id: "admin.colSigned",
      message: "Signed",
      comment: "Text in the organisation administration area.",
    }),
    download: i18n._({
      id: "admin.download",
      message: "Download",
      comment: "Text in the organisation administration area.",
    }),
    retentionTitle: i18n._({
      id: "admin.retentionTitle",
      message: "Retention",
      comment: "Text in the organisation administration area.",
    }),
    retentionCaption: i18n._({
      id: "admin.retentionCaption",
      message: "How long each data class is kept. A change waits for its notice period unless applied at once.",
      comment: "Text in the organisation administration area.",
    }),
    colClass: i18n._({
      id: "admin.colClass",
      message: "Data class",
      comment: "Text in the organisation administration area.",
    }),
    colDays: i18n._({ id: "admin.colDays", message: "Days", comment: "Text in the organisation administration area." }),
    colDefault: i18n._({
      id: "admin.colDefault",
      message: "Default",
      comment: "Text in the organisation administration area.",
    }),
    setDays: i18n._({
      id: "admin.setDays",
      message: "Days to keep",
      comment: "Text in the organisation administration area.",
    }),
    applyNow: i18n._({
      id: "admin.applyNow",
      message: "Apply at once",
      comment: "Text in the organisation administration area.",
    }),
    scheduledChanges: i18n._({
      id: "admin.scheduledChanges",
      message: "Scheduled changes",
      comment: "Text in the organisation administration area.",
    }),
    licenseTitle: i18n._({
      id: "admin.licenseTitle",
      message: "Licence",
      comment: "Text in the organisation administration area.",
    }),
    licenseCaption: i18n._({
      id: "admin.licenseCaption",
      message: "The enterprise licence of this installation, verified offline.",
      comment: "Text in the organisation administration area.",
    }),
    licenseState: i18n._({
      id: "admin.licenseState",
      message: "State",
      comment: "Text in the organisation administration area.",
    }),
    licenseId: i18n._({
      id: "admin.licenseId",
      message: "Licence ID",
      comment: "Text in the organisation administration area.",
    }),
    licenseOrg: i18n._({
      id: "admin.licenseOrg",
      message: "Licensed to",
      comment: "Text in the organisation administration area.",
    }),
    licenseFeatures: i18n._({
      id: "admin.licenseFeatures",
      message: "Features",
      comment: "Text in the organisation administration area.",
    }),
    licenseUpload: i18n._({
      id: "admin.licenseUpload",
      message: "Upload a licence",
      comment: "Text in the organisation administration area.",
    }),
    licenseFile: i18n._({
      id: "admin.licenseFile",
      message: "Licence file",
      comment: "Text in the organisation administration area.",
    }),
    licenseNote: i18n._({
      id: "admin.licenseNote",
      message: "Only installation administrators upload a licence. The server verifies it offline.",
      comment: "Text in the organisation administration area.",
    }),
    limitsTitle: i18n._({
      id: "admin.limitsTitle",
      message: "Limits and usage",
      comment: "Text in the organisation administration area.",
    }),
    limitsCaption: i18n._({
      id: "admin.limitsCaption",
      message: "The limits of this organisation. An override may lower a limit or raise it up to the maximum.",
      comment: "Text in the organisation administration area.",
    }),
    colLimit: i18n._({
      id: "admin.colLimit",
      message: "Limit",
      comment: "Text in the organisation administration area.",
    }),
    colOverride: i18n._({
      id: "admin.colOverride",
      message: "Override",
      comment: "Text in the organisation administration area.",
    }),
    colEffective: i18n._({
      id: "admin.colEffective",
      message: "Effective",
      comment: "Text in the organisation administration area.",
    }),
    colMax: i18n._({
      id: "admin.colMax",
      message: "Maximum",
      comment: "Text in the organisation administration area.",
    }),
    limitWarning: i18n._({
      id: "admin.limitWarning",
      message: "near the limit",
      comment: "Text in the organisation administration area.",
    }),
  };
}

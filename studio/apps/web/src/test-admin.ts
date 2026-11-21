// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, type ServiceImpl } from "@connectrpc/connect";
import type {
  AnalyticsService,
  AppService,
  AuditService,
  ComplianceService,
  FederationService,
  GovernanceService,
  IdentityService,
  InstallationService,
  LicenseService,
  OrgService,
} from "@plux/api";

/** The administration and installation areas (STU-116): what the RPCs return, and what they receive. */
export interface AdminData {
  members?: { userId: string; displayName: string; email: string; role: string }[];
  teams?: { id: string; key: string; name: string }[];
  roles?: { key: string; name: string; predefined?: boolean; permissions: string[] }[];
  permissions?: { name: string; description?: string; stepUp?: boolean }[];
  grants?: { id: string; userId?: string; teamId?: string; role: string; scope: string; appId?: string }[];
  restrictions?: {
    id: string;
    subjectKind: string;
    subjectId: string;
    patterns: string[];
    reason: string;
    appId?: string;
    environment?: string;
  }[];
  explain?: { decision: string; allowingGrantIds?: string[]; removingRestrictionIds?: string[]; ownerFloor?: boolean };
  tokens?: { id: string; name: string; prefix: string; revoked?: boolean }[];
  workloads?: { id: string; issuer: string; audience: string; subjectPattern: string; scopes: string[] }[];
  sessions?: { id: string; userId: string; clientKind: string }[];
  scimTokens?: { id: string; name: string; prefix: string }[];
  providers?: { id: string; name: string; kind: string; enabled: boolean }[];
  audit?: { id: string; action: string; actor: string; targetKind: string; targetId: string; atMs: number }[];
  destinations?: { id: string; name: string; kind: string; enabled: boolean }[];
  reports?: { id: string; changes: number; signed: boolean }[];
  retention?: { dataClass: string; days: number }[];
  license?: { state: string; licenseId?: string; organization?: string; features?: string[] };
  limits?: { key: string; unit: string; value: bigint; effective: bigint; hardMax: bigint }[];
  /** The organisation's apps (O32); when set, they answer ListApps. */
  apps?: { id: string; name: string }[];
  /** Each environment's secrets (O32), by environment id: what ListSecrets returns. */
  secrets?: Record<string, { key: string; hint: string }[]>;
  /** The secret the server returns once when a token or invitation is created. */
  secret?: string;
  installation?: {
    checks?: { name: string; healthy: boolean }[];
    storage?: { area: string; bytes: bigint }[];
    jobs?: { queue: string; state: string; count: bigint }[];
    issuers?: { issuer: string; audience: string; subjectPattern: string }[];
    organizations?: { id: string; name: string; createdAtMs: bigint; memberCount: bigint; appCount: bigint }[];
  };
  /** Every change the Studio sent, as `rpc:json` in order. */
  calls?: string[];
  /** Every read fails. */
  failing?: boolean;
  /** Reads the server refuses, by method name, with the Connect code and the Plux message (SRV-006). */
  refused?: Readonly<Record<string, readonly [Code, string]>>;
}

function recorder(a: AdminData) {
  return (name: string, payload: unknown) => {
    a.calls?.push(`${name}:${JSON.stringify(payload, (_k, v) => (typeof v === "bigint" ? v.toString() : v))}`);
  };
}

function guard(a: AdminData) {
  if (a.failing === true) {
    throw new ConnectError("unavailable", Code.Unavailable);
  }
}

/** A read the server refuses, as `refused` names it. */
export function refuse(a: AdminData, method: string) {
  const refusal = a.refused?.[method];
  if (refusal !== undefined) {
    throw new ConnectError(refusal[1], refusal[0]);
  }
}

const when = (ms: number) => timestampFromMs(ms);

export function orgAdmin(a: AdminData): Partial<ServiceImpl<typeof OrgService>> {
  const rec = recorder(a);
  return {
    getOrganization: (req) => ({ organization: { id: req.id, key: "alpha", name: "Alpha Org" } }),
    updateOrganization: (req) => {
      rec("updateOrganization", { id: req.id, name: req.name });
      return { organization: { id: req.id, name: req.name } };
    },
    listMembers: () => {
      guard(a);
      return { members: a.members ?? [] };
    },
    addMember: (req) => {
      rec("addMember", { email: req.email, role: req.role });
      return { member: { email: req.email, role: req.role }, invitation: a.secret ?? "" };
    },
    removeMember: (req) => {
      rec("removeMember", { userId: req.userId });
      return {};
    },
    listTeams: () => ({ teams: a.teams ?? [] }),
    createTeam: (req) => {
      refuse(a, "createTeam");
      rec("createTeam", { key: req.key, name: req.name });
      return { team: { key: req.key, name: req.name } };
    },
    deleteTeam: (req) => {
      rec("deleteTeam", { id: req.id });
      return {};
    },
    listOrganizationLimits: () => {
      guard(a);
      return { limits: a.limits ?? [] };
    },
    setOrganizationLimit: (req) => {
      rec("setOrganizationLimit", { key: req.key, value: req.value });
      return {};
    },
  };
}

export function governanceAdmin(a: AdminData): Partial<ServiceImpl<typeof GovernanceService>> {
  const rec = recorder(a);
  return {
    listRoles: () => {
      guard(a);
      return { roles: a.roles ?? [] };
    },
    getRole: (req) => ({ role: (a.roles ?? []).find((r) => r.key === req.key) }),
    createRole: (req) => {
      rec("createRole", { key: req.key, name: req.name, permissions: req.permissions });
      return {};
    },
    updateRole: (req) => {
      rec("updateRole", { key: req.key, name: req.name, permissions: req.permissions });
      return {};
    },
    deleteRole: (req) => {
      rec("deleteRole", { key: req.key });
      return {};
    },
    listPermissions: () => {
      guard(a);
      return { permissions: a.permissions ?? [] };
    },
    listGrants: () => ({ grants: a.grants ?? [] }),
    createGrant: (req) => {
      rec("createGrant", {
        userId: req.userId,
        teamId: req.teamId,
        role: req.role,
        scope: req.scope,
        appId: req.appId,
      });
      return {};
    },
    deleteGrant: (req) => {
      rec("deleteGrant", { id: req.id });
      return {};
    },
    listRestrictions: () => {
      guard(a);
      return { restrictions: a.restrictions ?? [] };
    },
    createRestriction: (req) => {
      rec("createRestriction", {
        kind: req.subjectKind,
        subject: req.subjectId,
        patterns: req.patterns,
        reason: req.reason,
        ...(req.appId !== "" && { app: req.appId }),
        ...(req.environment !== "" && { environment: req.environment }),
      });
      return {};
    },
    deleteRestriction: (req) => {
      rec("deleteRestriction", { id: req.id });
      return {};
    },
  };
}

/** O32: environments and their secrets; a value is recorded only as its length, as the server keeps no readable copy. */
export function appAdmin(a: AdminData): Partial<ServiceImpl<typeof AppService>> {
  const rec = recorder(a);
  return {
    ...(a.apps !== undefined && { listApps: () => ({ apps: a.apps ?? [] }) }),
    createEnvironment: (req) => {
      rec("createEnvironment", { appId: req.appId, key: req.key, name: req.name, production: req.production });
      return { environment: { id: `env-${req.key}`, appId: req.appId, key: req.key, name: req.name } };
    },
    listSecrets: (req) => {
      guard(a);
      return { secrets: a.secrets?.[req.environmentId] ?? [] };
    },
    setSecret: (req) => {
      refuse(a, "setSecret");
      rec("setSecret", { environmentId: req.environmentId, key: req.key, valueLength: req.value.length });
      return { secret: { key: req.key, hint: req.value.length < 8 ? "" : `…${req.value.slice(-4)}` } };
    },
    deleteSecret: (req) => {
      rec("deleteSecret", { environmentId: req.environmentId, key: req.key });
      return {};
    },
  };
}

export function identityAdmin(a: AdminData): Partial<ServiceImpl<typeof IdentityService>> {
  const rec = recorder(a);
  return {
    listAccessTokens: () => {
      guard(a);
      return { tokens: (a.tokens ?? []).map((t) => ({ ...t, revokedAt: t.revoked === true ? when(1) : undefined })) };
    },
    createAccessToken: (req) => {
      rec("createAccessToken", { name: req.name, scopes: req.scopes, ttlSeconds: req.ttlSeconds });
      return { token: { id: "t-new", name: req.name }, secret: a.secret ?? "" };
    },
    revokeAccessToken: (req) => {
      rec("revokeAccessToken", { id: req.id });
      return {};
    },
    listWorkloadIdentities: () => ({ identities: a.workloads ?? [] }),
    createWorkloadIdentity: (req) => {
      rec("createWorkloadIdentity", { issuer: req.issuer, audience: req.audience, subject: req.subjectPattern });
      return {};
    },
    deleteWorkloadIdentity: (req) => {
      rec("deleteWorkloadIdentity", { id: req.id });
      return {};
    },
    listSessions: () => {
      guard(a);
      refuse(a, "listSessions");
      return { sessions: a.sessions ?? [] };
    },
    revokeSession: (req) => {
      rec("revokeSession", { id: req.id });
      return {};
    },
    listAuditEntries: (req) => {
      guard(a);
      rec("listAuditEntries", { filter: req.page?.filter ?? "", orderBy: req.page?.orderBy ?? "" });
      const entries = req.page?.orderBy === "sequence desc" ? [...(a.audit ?? [])].reverse() : (a.audit ?? []);
      return {
        entries: entries.map((e) => ({
          id: e.id,
          action: e.action,
          actor: { display: e.actor },
          targetKind: e.targetKind,
          targetId: e.targetId,
          time: when(e.atMs),
        })),
      };
    },
  };
}

export function federationAdmin(a: AdminData): Partial<ServiceImpl<typeof FederationService>> {
  const rec = recorder(a);
  return {
    listIdentityProviders: () => {
      guard(a);
      return { providers: a.providers ?? [] };
    },
    listScimTokens: () => {
      refuse(a, "listScimTokens");
      return { tokens: a.scimTokens ?? [] };
    },
    createScimToken: (req) => {
      rec("createScimToken", { name: req.name });
      return { token: { id: "s-new", name: req.name }, secret: a.secret ?? "" };
    },
    rotateScimToken: (req) => {
      rec("rotateScimToken", { id: req.id });
      return { token: { id: req.id }, secret: a.secret ?? "" };
    },
    revokeScimToken: (req) => {
      rec("revokeScimToken", { id: req.id });
      return {};
    },
  };
}

export function complianceAdmin(a: AdminData): Partial<ServiceImpl<typeof ComplianceService>> {
  const rec = recorder(a);
  return {
    listComplianceReports: () => {
      guard(a);
      refuse(a, "listComplianceReports");
      return { reports: a.reports ?? [] };
    },
    createComplianceReport: (req) => {
      rec("createComplianceReport", { from: req.from?.seconds, to: req.to?.seconds });
      return {};
    },
  };
}

export function auditAdmin(a: AdminData): Partial<ServiceImpl<typeof AuditService>> {
  const rec = recorder(a);
  return {
    listExportDestinations: () => {
      refuse(a, "listExportDestinations");
      return { destinations: a.destinations ?? [] };
    },
    createExportDestination: (req) => {
      // What the server is sent, with the credentials reduced to whether they were sent at all.
      rec("createExportDestination", {
        name: req.name,
        kind: req.kind,
        address: req.address,
        allowPrivate: req.allowPrivate,
        clientKey: req.clientKeyPem !== "",
        signingSecret: req.signingSecret !== "",
      });
      a.destinations = [
        ...(a.destinations ?? []),
        { id: `d${(a.destinations?.length ?? 0) + 1}`, name: req.name, kind: req.kind, enabled: true },
      ];
      return {};
    },
  };
}

export function analyticsAdmin(a: AdminData): Partial<ServiceImpl<typeof AnalyticsService>> {
  const rec = recorder(a);
  return {
    getRetention: () => {
      guard(a);
      return { settings: (a.retention ?? []).map((r) => ({ ...r, isDefault: true })) };
    },
    setRetention: (req) => {
      rec("setRetention", { dataClass: req.dataClass, days: req.days, immediate: req.immediate });
      return {};
    },
  };
}

export function licenseAdmin(a: AdminData): Partial<ServiceImpl<typeof LicenseService>> {
  const rec = recorder(a);
  return {
    getLicenseState: () => {
      guard(a);
      return a.license ?? { state: "absent" };
    },
    installLicense: (req) => {
      rec("installLicense", { bytes: req.license.length });
      return { state: { state: "valid" } };
    },
  };
}

export function installationAdmin(a: AdminData): Partial<ServiceImpl<typeof InstallationService>> {
  const i = a.installation ?? {};
  return {
    getInstallationHealth: () => {
      guard(a);
      return { checks: i.checks ?? [] };
    },
    getInstallationStorage: () => ({ areas: i.storage ?? [] }),
    listInstallationJobs: () => ({ jobs: i.jobs ?? [] }),
    listTrustedIssuers: () => ({ issuers: i.issuers ?? [] }),
    listInstallationOrganizations: () => ({ organizations: i.organizations ?? [] }),
  };
}

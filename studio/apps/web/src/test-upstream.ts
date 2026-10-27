// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport, type ServiceImpl } from "@connectrpc/connect";
import {
  AnalyticsService,
  type ApplyChangeSetRequest,
  ApprovalRequestSchema,
  ApprovalService,
  AppService,
  AuditService,
  BreakGlassService,
  ComplianceService,
  ComponentService,
  ControlSchema,
  ControlService,
  DevDeviceSchema,
  type DevEvent,
  DeviceSchema,
  DeviceService,
  DevSessionService,
  type DevSessionServiceSendCommandRequest,
  DocumentSchema,
  DocumentService,
  ExperimentService,
  FederationService,
  FlagService,
  FunctionAdminService,
  GetCurrentUserResponseSchema,
  GovernanceService,
  IdentityService,
  InboxItemSchema,
  InstallationService,
  LanguageServerService,
  LicenseService,
  LockSchema,
  ManifestService,
  MemberSchema,
  NotificationService,
  OrganizationSchema,
  OrgService,
  PluginSchema,
  PluginService,
  PluginVersionSchema,
  PreferencesSchema,
  PreferencesService,
  PublishService,
  ReleaseRequestService,
  ReleaseSchema,
  ReleaseService,
  RolloutSchema,
  RolloutService,
  SecurityAdminService,
  TemplateService,
  UserSchema,
} from "@plux/api";
import { applyPatch, type JsonPatchOp, type JsonValue } from "./features/editor/json-patch.ts";
import {
  type AdminData,
  analyticsAdmin,
  auditAdmin,
  complianceAdmin,
  federationAdmin,
  governanceAdmin,
  identityAdmin,
  installationAdmin,
  licenseAdmin,
  orgAdmin,
} from "./test-admin.ts";

export const orgA = "0192f000-0000-7000-8000-00000000000a";
export const orgB = "0192f000-0000-7000-8000-00000000000b";

/** The Devices and security area (STU-115): what the device, security and manifest RPCs return, and what they receive. */
export interface SecurityData {
  devices?: { id: string; platform: string; hostBuild: string; assuranceLevel: string; revoked?: boolean }[];
  profile?: string;
  version?: number;
  settings?: Record<string, unknown>;
  keys?: { keyId: string; role: string; algorithm: string; environmentType: string }[];
  roots?: number;
  revoked?: { deviceId: string; reason: string }[];
  /** The overrides the environment stores, as the object the server returns. */
  overrides?: Record<string, unknown>;
  profiles?: { profile: string; environmentId: string; expectedVersion: bigint; overrides: string }[];
  /** The step-up RPCs the Studio called, in order. */
  stepUps?: string[];
  putDocuments?: { path: string; content: string; ifRevision: bigint }[];
  /** Every read fails. */
  failing?: boolean;
}

/** The Approvals area (STU-102): requests with their evidence, break-glass reviews, policies and the simulator. */
export interface ApprovalsData {
  requests?: {
    id: string;
    appId: string;
    status: string;
    requestedBy: string;
    summary: string;
    subject?: string;
    resource?: string;
    approvalsRequired?: number;
    diff?: string;
    risk?: string;
    decisions?: { id: string; decider: string; decision: string; comment: string; auditSequence?: number }[];
  }[];
  breakGlass?: {
    id: string;
    action: string;
    subject: string;
    invokedBy: string;
    reviewStatus: string;
    overdue?: boolean;
  }[];
  policies?: { key: string; name: string; approvers: number }[];
  /** Collects every DecideApproval the upstream receives. */
  decided?: { appId: string; id: string; decision: string; comment: string }[];
  /** Collects every ReviewBreakGlass. */
  reviewed?: { id: string; comment: string }[];
  /** Collects every PutPolicy document. */
  saved?: string[];
  /** Collects the draft and the sample of every SimulatePolicy. */
  simulated?: { document: string; resource: string; environment: string }[];
  /** DecideApproval and ReviewBreakGlass refuse with this message (the author deciding their own request, PLX-8003). */
  refusal?: string;
  failing?: boolean;
}

/** The library: ListLibrary*, versions, visibility, export/import, usages and adoptions (STU-080 to STU-086). */
export interface LibraryFixture {
  items: {
    id: string;
    kind: "component" | "template";
    key: string;
    name: string;
    description?: string;
    visibility: string;
    headVersion?: number;
    templateKind?: string;
    documentId?: string;
    createdByName?: string;
  }[];
  versions?: Record<string, { version: number; content?: string; note?: string }[]>;
  parameters?: { name: string; type: string; required: boolean }[];
  /** Instances per app that ListUsages reports. */
  usages?: number;
  /** Use sites that ListUsages reports, instead of \`usages\` anonymous ones. */
  sites?: { file: string; path: string }[];
  /** Public scan refusal: the message SetVisibility fails with. */
  scanRefusal?: string;
  conflicts?: { file: string; path: string; removedProps: string[] }[];
  /** Records \`rpc:detail\` strings. */
  calls?: string[];
  failList?: boolean;
}

/** The publish flow (STU-100): one release request that moves through the states, and what the Studio sent. */
export interface ReleaseFixture {
  request: {
    id: string;
    state: string;
    hold?: string;
    createdBy?: string;
    createdByName?: string;
    plugins: { pluginKey: string; reason: string }[];
    warnings?: string[];
    acknowledged?: string[];
    errors?: number;
  };
  /** The visual diff job, when one was requested. */
  diff?: { state: string };
  /** The RPCs received, in order, with the step they completed. */
  calls: string[];
  /** The idempotency keys of the publish jobs. */
  keys?: string[];
  /** The approval ends rejected instead of moving on. */
  rejectApproval?: boolean;
}

const releaseOrder = ["select", "build", "reports", "diffs", "changelog", "target", "approval", "rollout", "release"];

/** The Releases area (STU-101): releases, their rollouts, the kill switches, and every mutation received as \`rpc:detail\`. */
export interface ReleasesFixture {
  releases: { sequence: number; notes?: string; rollbackOf?: number }[];
  rollouts?: {
    id: string;
    candidate: number;
    state: number;
    basisPoints: number;
    stages: number[];
    stageIndex?: number;
    lastCheck?: unknown;
  }[];
  killed?: string[];
  calls?: string[];
  failing?: boolean;
}

/** The Experiments and flags area (STU-103): experiments with results, flags, and every mutation received as \`rpc:detail\`. */
export interface ExperimentsFixture {
  experiments: { id: string; key: string; state: number; definition?: unknown; winner?: string }[];
  flags?: { key: string; type: number; defaultJson: string; description?: string }[];
  calls?: string[];
  failing?: boolean;
}

/** The functions of the Functions area (STU-110): versions, aliases, metrics, logs and stored-test results. */
export interface FunctionsFixture {
  functions: { name: string; version: number; placement?: string; testsPassed?: boolean; signed?: boolean }[];
  aliases?: { name: string; version: number }[];
  metrics?: { invocations: number; errorsByKind?: Record<string, number>; p95?: number };
  logs?: { level: string; message: string; version?: number }[];
  tests?: { results: { name: string; passed: boolean; message: string }[] };
  /** Collects every SetAlias as `alias:version`. */
  calls?: string[];
  failing?: boolean;
}

export interface UpstreamData {
  experiments?: ExperimentsFixture;
  release?: ReleaseFixture;
  releases?: ReleasesFixture;
  functions?: FunctionsFixture;
  security?: SecurityData;
  approvals?: ApprovalsData;
  /** The administration and installation areas (STU-116). */
  admin?: AdminData;
  /** The dev session (DEV-002, DEV-011, DEV-020): paired devices, the pairing token, the event stream and what Studio sent. */
  devSession?: {
    devices: { id: string; name: string; connected: boolean }[];
    pairing?: { uri: string; code: string; expiresInSeconds: number };
    events?: DevEvent[];
    /** When set, the stream yields its events only after this resolves (a test orders them against Studio's own steps). */
    gate?: Promise<void>;
    /** Collects every SendCommand. */
    commands?: DevSessionServiceSendCommandRequest[];
    /** Collects the device filter of every ExportTrace. */
    exported?: string[][];
  };
  library?: LibraryFixture;
  signedIn?: boolean;
  permissions?: string[];
  installationAdmin?: boolean;
  csrfToken?: string;
  memberships?: string[];
  stored?: { theme: string; language: string; density: string; mutedNotificationTypes?: string[] };
  explained?: { permission: string }[];
  inbox?: { id: string; eventType: string; title?: string; link?: string; read: boolean }[];
  documents?: { kind: string; path: string; json: unknown }[];
  /** Someone else holds the plugin's editing lock: the editor is read-only. */
  lockHeldElsewhere?: boolean;
  /** Collects every PatchDocument the upstream receives (autosave, undo, redo). */
  sent?: { path: string; pluginId: string; stepId: string; ops: { op: string; path: string; value?: unknown }[] }[];
  /** Collects every lock taken, by the plugin it names (empty for the app's own lock) and its scope. */
  acquired?: { pluginId: string; scope: string }[];
  /** What ListDocuments returns for the whole app (no plugin); defaults to `documents`. */
  /** The app's plugins (ids) and their documents, for ListPlugins and ListDocuments. */
  pluginDocuments?: Record<string, { kind: string; path: string; json: unknown }[]>;
  appDocuments?: { kind: string; path: string; json: unknown }[];
  /** A production channel serves a release (STU-074). */
  productionRelease?: boolean;
  /** Collects every ApplyChangeSet the upstream receives. */
  changeSets?: ApplyChangeSetRequest[];
  /** The documents of the plugin's last published version (ListPluginVersions, GetSnapshot); none means never published. */
  published?: { path: string; json: unknown }[];
  /** The app canvas: ListPluginSummaries. */
  summaries?: {
    id: string;
    key: string;
    name: string;
    latestVersion?: number;
    bundleSize?: number;
    screens?: number;
    versions?: number;
    validationErrors?: number;
    functionsUsed?: string[];
    unpublishedChanges?: boolean;
  }[];
  /** The plugin's history: ListSnapshots, GetSnapshot, CompareSnapshots and RestoreSnapshot (STU-069). */
  snapshots?: {
    id: string;
    revision: number;
    reason: string;
    author: string;
    editorSession?: string;
    documents: { path: string; json: unknown }[];
  }[];
  /** Collects the RestoreSnapshot calls. */
  restoredSnapshots?: { id: string; paths: string[] }[];
  /** Collects plugin RPCs the app canvas sends, as `delete:<id>` and `archive:<id>`. */
  pluginCalls?: string[];
  /** The app's trash, and the ids restored from it. */
  trash?: { id: string; kind: string; name: string }[];
  restored?: string[];
  /** A language server and a test runner behind the Rust editor (FN-033, FN-034); absent means the RPCs are unimplemented. */
  rust?: {
    /** Answers one client message (JSON text) with the messages the server sends back. */
    reply: (message: string) => string[];
    /** Every message the language server received, in order. */
    received?: string[];
    /** The test cases the runner reports, and the summary. */
    tests?: { results: { name: string; passed: boolean; message: string }[]; diagnostics?: string };
  };
  updates?: { paths: string[]; theme: string; language: string; density: string; mutedNotificationTypes?: string[] }[];
}

type LibraryItems = NonNullable<UpstreamData["library"]>;

function libraryList(lib: LibraryItems | undefined, kind: string, visibility: string, query: string) {
  if (lib?.failList === true) {
    throw new ConnectError("down", Code.Unavailable);
  }
  const items = (lib?.items ?? []).filter(
    (i) => i.kind === kind && (visibility === "" || i.visibility === visibility) && i.name.includes(query),
  );
  return { items: items.map((i) => ({ headVersion: 1, description: "", createdByName: "", ...i })) };
}

function libraryVersions(lib: LibraryItems | undefined, id: string) {
  const item = lib?.items.find((i) => i.id === id);
  if (item === undefined) {
    throw new ConnectError("missing", Code.NotFound);
  }
  return {
    item: { headVersion: 1, description: "", ...item },
    versions: (lib?.versions?.[id] ?? [{ version: 1 }]).map((v) => ({
      itemId: id,
      createdByName: "Ada",
      note: v.note ?? "",
      version: v.version,
      content: new TextEncoder().encode(v.content ?? "{}"),
    })),
  };
}

function libraryVisibility(lib: LibraryItems | undefined, kind: string, id: string, visibility: string) {
  lib?.calls?.push(`visibility:${kind}:${id}:${visibility}`);
  if (visibility === "public" && lib?.scanRefusal !== undefined) {
    throw new ConnectError(lib.scanRefusal, Code.FailedPrecondition);
  }
  return {};
}

/** An in-memory upstream for the shell: the same RPCs the BFF forwards. */
export function createUpstream(state: UpstreamData) {
  // Documents autosave changed, by path; the fixtures themselves stay as given.
  const patched = new Map<string, { kind: string; path: string; json: unknown }>();
  const stored = state.stored ?? { theme: "", language: "", density: "" };
  return createRouterTransport(({ service }) => {
    const admin = state.admin ?? {};
    service(IdentityService, {
      ...identityAdmin(admin),
      beginStepUp: () => {
        state.security?.stepUps?.push("begin");
        return { requestId: "su-1", optionsJson: JSON.stringify({ challenge: "AQ", userVerification: "required" }) };
      },
      completeStepUp: (req) => {
        state.security?.stepUps?.push(`complete:${req.requestId}`);
        return {};
      },
      getCurrentUser: () => {
        if (state.signedIn === false) {
          throw new ConnectError("no session", Code.Unauthenticated);
        }
        return create(GetCurrentUserResponseSchema, {
          user: create(UserSchema, { id: "u1", displayName: "Ada", email: "ada@example.org" }),
          memberships: (state.memberships ?? [orgA, orgB]).map((id) =>
            create(MemberSchema, { organizationId: id, userId: "u1" }),
          ),
          permissions: state.permissions ?? [],
          installationAdmin: state.installationAdmin ?? false,
          csrfToken: state.csrfToken ?? "",
        });
      },
    });
    service(OrgService, {
      ...orgAdmin(admin),
      listOrganizations: () => ({
        organizations: [
          create(OrganizationSchema, { id: orgA, key: "alpha", name: "Alpha Org" }),
          create(OrganizationSchema, { id: orgB, key: "beta", name: "Beta Org" }),
        ],
      }),
    });
    service(PreferencesService, {
      getPreferences: () => ({ preferences: create(PreferencesSchema, stored) }),
      updatePreferences: (req) => {
        Object.assign(stored, { ...req.preferences });
        state.updates?.push({ paths: req.updateMask?.paths ?? [], ...stored });
        return { preferences: create(PreferencesSchema, stored) };
      },
    });
    service(GovernanceService, {
      ...governanceAdmin(admin),
      explainAccess: (req) => {
        state.explained?.push({ permission: req.permission });
        return { decision: "deny", ...admin.explain };
      },
    });
    service(NotificationService, {
      listInbox: (req) => {
        const items = (state.inbox ?? []).filter((i) => !req.unreadOnly || !i.read);
        return {
          items: items.map((i) => create(InboxItemSchema, i)),
          unread: BigInt(items.filter((i) => !i.read).length),
        };
      },
    });
    service(AppService, {
      getApp: () => ({ app: { id: "app1", name: "App", defaultPluginKey: "" } }),
      listEnvironments: () => ({
        environments: [
          { id: "env-prod", key: "production", production: true },
          { id: "env-dev", key: "development", production: false },
        ],
      }),
      listChannels: () => ({
        channels: [
          {
            id: "ch",
            environmentId: "env-prod",
            key: "production",
            releaseSequence: state.productionRelease === true ? 3n : 0n,
          },
        ],
      }),
      listApps: () => ({ apps: [{ id: "app1", key: "shop", name: "Shop" }] }),
      listTrash: () => ({ items: state.trash ?? [] }),
      restoreFromTrash: (req) => {
        state.restored?.push(req.id);
        return {};
      },
    });
    service(TemplateService, {
      listLibraryTemplates: (req) => libraryList(state.library, "template", req.visibility, req.query),
      listTemplateVersions: (req) => libraryVersions(state.library, req.id),
      getTemplate: (req) => ({
        template: { id: req.id, name: req.id, parameters: state.library?.parameters ?? [] },
      }),
      createTemplate: (req) => {
        state.library?.calls?.push(`createTemplate:${req.draft?.key ?? ""}`);
        return { item: { id: "newt" }, version: { version: 1 } };
      },
      instantiate: (req) => {
        state.library?.calls?.push(`instantiate:${req.templateId}:${req.key}:${req.session}`);
        return { created: [`plugins/p/pages/${req.key}.page.json`], diagnostics: [] };
      },
      setTemplateVisibility: (req) => libraryVisibility(state.library, "template", req.id, req.visibility),
      exportTemplate: (req) => {
        state.library?.calls?.push(`exportTemplate:${req.id}:${req.version}`);
        return { file: new TextEncoder().encode("{}"), fileName: `${req.id}.plux-template` };
      },
      importTemplate: (req) => {
        state.library?.calls?.push(`importTemplate:${req.file.length}`);
        return { item: { id: "imported", kind: "template", name: "Imported template" } };
      },
    });
    service(ComponentService, {
      listLibraryComponents: (req) => libraryList(state.library, "component", req.visibility, req.query),
      listComponentVersions: (req) => libraryVersions(state.library, req.id),
      setComponentVisibility: (req) => libraryVisibility(state.library, "component", req.id, req.visibility),
      exportComponent: (req) => {
        state.library?.calls?.push(`exportComponent:${req.id}:${req.version}`);
        return { file: new TextEncoder().encode("{}"), fileName: `${req.id}.plux-component` };
      },
      importComponent: (req) => {
        state.library?.calls?.push(`importComponent:${req.file.length}`);
        return { item: { id: "imported", kind: "component", name: "Imported component" } };
      },
      listUsages: () => ({
        usages:
          state.library?.sites?.map((l) => ({ kind: "node", pluginKey: "p", location: l })) ??
          Array.from({ length: state.library?.usages ?? 0 }, () => ({ kind: "node", pluginKey: "p" })),
      }),
      reviseComponent: (req) => {
        state.library?.calls?.push(`revise:${req.id}:${req.expectedVersion}:${req.draft?.content.length ?? 0}`);
        return { item: { id: req.id }, version: { version: req.expectedVersion + 1 } };
      },
      createComponent: (req) => {
        state.library?.calls?.push(`createComponent:${req.draft?.key ?? ""}`);
        return { item: { id: "newc" }, version: { version: 1 } };
      },
      listComponentAdoptions: () => ({ adoptions: [] }),
      adoptComponentVersion: (req) => {
        state.library?.calls?.push(`adopt:${req.appId}:${req.version}`);
        return { conflicts: state.library?.conflicts ?? [] };
      },
    });
    service(PluginService, {
      listPluginSummaries: () => ({
        summaries: (state.summaries ?? []).map((p) => ({
          plugin: { id: p.id, key: p.key, name: p.name, latestVersion: BigInt(p.latestVersion ?? 0) },
          bundleSize: BigInt(p.bundleSize ?? 0),
          screens: BigInt(p.screens ?? 0),
          versions: BigInt(p.versions ?? 0),
          validationErrors: BigInt(p.validationErrors ?? 0),
          functionsUsed: p.functionsUsed ?? [],
          unpublishedChanges: p.unpublishedChanges ?? false,
        })),
      }),
      deletePlugin: (req) => {
        state.pluginCalls?.push(`delete:${req.id}`);
        return {};
      },
      archivePlugin: (req) => {
        state.pluginCalls?.push(`archive:${req.id}`);
        return {};
      },
      listPlugins: () => ({
        plugins: Object.keys(state.pluginDocuments ?? {}).map((id) => create(PluginSchema, { id, key: id })),
      }),
      // The editing lock: free until taken, then held by the session that took it.
      getLock: (req) =>
        state.lockHeldElsewhere === true
          ? {
              lock: create(LockSchema, {
                appId: req.appId,
                pluginId: req.pluginId,
                session: "another-tab",
                holder: { kind: "user", id: "someone-else", display: "Ana" },
                expiresAt: { seconds: BigInt(Math.floor(Date.now() / 1000) + 3600), nanos: 0 },
              }),
            }
          : {},
      acquireLock: (req) => {
        state.acquired?.push({ pluginId: req.pluginId, scope: req.scope });
        if (state.lockHeldElsewhere === true) {
          throw new ConnectError("the lock is held by someone else", Code.FailedPrecondition);
        }
        return {
          lock: create(LockSchema, {
            appId: req.appId,
            pluginId: req.pluginId,
            session: req.session,
            expiresAt: { seconds: BigInt(Math.floor(Date.now() / 1000) + 3600), nanos: 0 },
          }),
        };
      },
      renewLock: (req) => ({
        lock: create(LockSchema, {
          appId: req.appId,
          pluginId: req.pluginId,
          session: req.session,
          expiresAt: { seconds: BigInt(Math.floor(Date.now() / 1000) + 3600), nanos: 0 },
        }),
      }),
      releaseLock: () => ({}),
    });
    if (state.release !== undefined) {
      service(ReleaseRequestService, releaseRequestHandlers(state.release));
      service(PublishService, {
        publish: (req, ctx) => {
          state.release?.keys?.push(ctx.requestHeader.get("Idempotency-Key") ?? "");
          return { job: { id: `job-${req.pluginId}`, state: "running", percent: 40 } };
        },
        getPublishJob: (req) => ({ job: { id: req.jobId, state: "succeeded", percent: 100, version: 1n } }),
      });
    }
    service(ReleaseService, {
      ...releaseHandlers(state.releases),
      listPluginVersions: () => ({
        versions:
          state.published === undefined
            ? []
            : [
                create(PluginVersionSchema, { id: "v1", version: 1n, sourceSnapshotId: "snap-1" }),
                create(PluginVersionSchema, { id: "v0", version: 0n, sourceSnapshotId: "snap-0" }),
              ],
      }),
    });
    service(DocumentService, {
      listSnapshots: () => ({
        snapshots: (state.snapshots ?? []).map((x) => ({
          id: x.id,
          revision: BigInt(x.revision),
          reason: x.reason,
          actor: { kind: "user", id: "u", display: x.author },
          editorSessionId: x.editorSession ?? "",
          createdAt: { seconds: BigInt(1_700_000_000 + x.revision * 60), nanos: 0 },
        })),
      }),
      compareSnapshots: (req) => {
        const find = (id: string) => (state.snapshots ?? []).find((x) => x.id === id)?.documents ?? [];
        const [a, b] = [find(req.fromId), find(req.toId)];
        const paths = [...new Set([...a, ...b].map((d) => d.path))].sort();
        return {
          changes: paths.flatMap((path) => {
            const x = a.find((d) => d.path === path);
            const y = b.find((d) => d.path === path);
            const change = x === undefined ? "added" : y === undefined ? "removed" : "changed";
            return JSON.stringify(x?.json) === JSON.stringify(y?.json) ? [] : [{ path, change, patch: [] }];
          }),
        };
      },
      restoreSnapshot: (req) => {
        state.restoredSnapshots?.push({ id: req.id, paths: [...req.paths] });
        return { snapshotId: "restored" };
      },
      getSnapshot: (req) => ({
        documents: (req.id === "snap-1"
          ? (state.published ?? [])
          : ((state.snapshots ?? []).find((x) => x.id === req.id)?.documents ?? [])
        )
          .filter((d) => req.paths.length === 0 || req.paths.includes(d.path))
          .map((d) =>
            create(DocumentSchema, { path: d.path, content: new TextEncoder().encode(JSON.stringify(d.json)) }),
          ),
      }),
      // Autosave: applies the patch to the stored document and returns it one revision on.
      patchDocument: (req) => {
        const stored =
          patched.get(req.path) ??
          [...(state.documents ?? []), ...(state.appDocuments ?? [])].find((d) => d.path === req.path);
        const doc = stored === undefined ? undefined : { ...stored };
        const ops = req.patch.map((o) => ({
          op: o.op as JsonPatchOp["op"],
          path: o.path,
          ...(o.value.length > 0 && { value: JSON.parse(new TextDecoder().decode(o.value)) as JsonValue }),
        }));
        state.sent?.push({ path: req.path, pluginId: req.pluginId, stepId: req.stepId, ops });
        if (doc !== undefined) {
          doc.json = applyPatch(doc.json as JsonValue, ops);
          patched.set(req.path, doc);
        }
        return {
          document: create(DocumentSchema, {
            kind: doc?.kind ?? "",
            path: req.path,
            revision: req.ifRevision + 1n,
            content: new TextEncoder().encode(JSON.stringify(doc?.json ?? {})),
          }),
        };
      },
      applyChangeSet: (req) => {
        state.changeSets?.push(req);
        return {};
      },
      // The document a restore brought back: found in the snapshots by path.
      putDocument: (req) => {
        state.security?.putDocuments?.push({
          path: req.path,
          content: new TextDecoder().decode(req.content),
          ifRevision: req.ifRevision,
        });
        return {};
      },
      getDocument: (req) => {
        const found = [
          ...(state.snapshots ?? []).flatMap((x) => x.documents),
          ...(req.pluginId === "" ? (state.appDocuments ?? []) : []),
        ].find((d) => d.path === req.path);
        return {
          document:
            found === undefined
              ? undefined
              : create(DocumentSchema, {
                  pluginId: req.pluginId,
                  path: found.path,
                  revision: 1n,
                  content: new TextEncoder().encode(JSON.stringify(found.json)),
                }),
        };
      },
      listDocuments: (req) => ({
        documents: (req.pluginId === ""
          ? (state.appDocuments ?? state.documents ?? [])
          : (state.pluginDocuments?.[req.pluginId] ?? state.documents ?? [])
        ).map((d) =>
          create(DocumentSchema, {
            pluginId: req.pluginId,
            kind: d.kind,
            path: d.path,
            content: new TextEncoder().encode(JSON.stringify(d.json)),
          }),
        ),
      }),
    });
    if (state.devSession !== undefined) {
      devSessionService(service, state.devSession);
    }
    if (state.rust !== undefined) {
      rustServices(service, state.rust);
    }
    securityServices(service, state.security ?? {});
    if (state.releases !== undefined) {
      releasesServices(service, state.releases);
    }
    if (state.functions !== undefined) {
      functionsServices(service, state.functions);
    }
    if (state.experiments !== undefined) {
      experimentsServices(service, state.experiments);
    }
    approvalsServices(service, state.approvals ?? {});
    service(FederationService, { ...federationAdmin(admin), listSignInProviders: () => ({ providers: [] }) });
    service(ComplianceService, complianceAdmin(admin));
    service(AuditService, auditAdmin(admin));
    service(AnalyticsService, analyticsAdmin(admin));
    service(LicenseService, licenseAdmin(admin));
    service(InstallationService, installationAdmin(admin));
  });
}

type DevSessionData = NonNullable<UpstreamData["devSession"]>;

/** The dev session service in memory: pairing, devices, commands, a fixed event stream and the trace export. */
function devSessionService(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  dev: DevSessionData,
) {
  service(DevSessionService, {
    createSession: () => ({ session: { id: "s1" } }),
    getPairing: () => ({
      pairing: {
        token: `token-${Date.now()}`,
        uri: dev.pairing?.uri ?? "plux-dev://pair?server=https%3A%2F%2Fstudio.example&token=t",
        code: dev.pairing?.code ?? "ABCD-2345",
        expiresAt: timestampFromMs(Date.now() + (dev.pairing?.expiresInSeconds ?? 120) * 1000),
      },
    }),
    listDevices: () => ({ devices: dev.devices.map((d) => create(DevDeviceSchema, d)) }),
    sendCommand: (req) => {
      dev.commands?.push(req);
      return { commandId: "c1", deviceIds: req.deviceId === "" ? dev.devices.map((d) => d.id) : [req.deviceId] };
    },
    async *watchEvents(req, ctx) {
      await dev.gate;
      for (const event of dev.events ?? []) {
        if (req.deviceIds.length === 0 || req.deviceIds.includes(event.deviceId)) {
          yield { event };
        }
      }
      await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve()));
    },
    exportTrace: (req) => {
      dev.exported?.push([...req.deviceIds]);
      return { trace: new Uint8Array([31, 139]), eventCount: 0n };
    },
  });
}

type Rust = NonNullable<UpstreamData["rust"]>;

/** The language-server stream and the test runner of the Rust editor, in memory. */
function rustServices(service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"], rust: Rust) {
  const outbox: Uint8Array[] = [];
  let wake: (() => void) | undefined;
  service(LanguageServerService, {
    async *open() {
      yield {
        event: {
          case: "opened",
          value: { sessionId: "s1", rootUri: "file:///workspace/functions/x/", sessionSeconds: 60 },
        },
      };
      for (;;) {
        const next = outbox.shift();
        if (next !== undefined) {
          yield { event: { case: "message", value: next } };
        } else {
          await new Promise<void>((resolve) => {
            wake = resolve;
          });
        }
      }
    },
    send(req) {
      const text = new TextDecoder().decode(req.message);
      rust.received?.push(text);
      for (const r of rust.reply(text)) {
        outbox.push(new TextEncoder().encode(r));
      }
      wake?.();
      return {};
    },
  });
  service(FunctionAdminService, {
    async *runTests() {
      for (const r of rust.tests?.results ?? []) {
        yield { event: { case: "result", value: r } };
      }
      const results = rust.tests?.results ?? [];
      yield {
        event: {
          case: "finished",
          value: {
            passed: results.filter((r) => r.passed).length,
            failed: results.filter((r) => !r.passed).length,
            diagnostics: new TextEncoder().encode(rust.tests?.diagnostics ?? ""),
          },
        },
      };
    },
  });
}

/** The device, security-admin and manifest RPCs of the Devices and security area, in memory. */
function securityServices(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  data: SecurityData,
) {
  const fail = () => {
    if (data.failing === true) {
      throw new ConnectError("unavailable", Code.Unavailable);
    }
  };
  const device = (d: NonNullable<SecurityData["devices"]>[number]) =>
    create(DeviceSchema, {
      id: d.id,
      platform: d.platform,
      hostBuild: d.hostBuild,
      assuranceLevel: d.assuranceLevel,
      ...(d.revoked === true && { revokedAt: { seconds: 1n, nanos: 0 } }),
    });
  service(DeviceService, {
    listDevices: () => {
      fail();
      return { devices: (data.devices ?? []).map(device) };
    },
    getDevice: (req) => {
      fail();
      const found = (data.devices ?? []).find((d) => d.id === req.id);
      return { device: found === undefined ? undefined : device(found) };
    },
    revokeDevice: (req) => {
      data.revoked?.push({ deviceId: req.deviceId, reason: req.reason });
      const found = (data.devices ?? []).find((d) => d.id === req.deviceId);
      if (found !== undefined) {
        found.revoked = true;
      }
      return { device: found === undefined ? undefined : device(found) };
    },
  });
  service(SecurityAdminService, {
    getEffectiveSecurityConfig: () => {
      fail();
      return {
        profile: data.profile ?? "standard",
        version: BigInt(data.version ?? 1),
        effectiveJson: new TextEncoder().encode(JSON.stringify(data.settings ?? {})),
        overridesJson: new TextEncoder().encode(JSON.stringify(data.overrides ?? {})),
      };
    },
    setSecurityConfig: (req) => {
      data.profiles?.push({
        profile: req.profile,
        environmentId: req.environmentId,
        expectedVersion: req.expectedVersion,
        overrides: new TextDecoder().decode(req.overridesJson),
      });
      data.profile = req.profile;
      return { version: req.expectedVersion + 1n };
    },
  });
  service(ManifestService, {
    getRootKeys: () => {
      fail();
      return { keys: data.keys ?? [], roots: Array.from({ length: data.roots ?? 0 }, () => new Uint8Array()) };
    },
  });
}

const releaseHash = "c0ffee00c0ffee00c0ffee00";

function refused(code: string): never {
  throw new ConnectError(`${code} refused`, Code.FailedPrecondition);
}

/** The release request state machine of ReleaseRequestService, as far as the Studio needs it. */
function releaseRequestHandlers(fx: ReleaseFixture): Partial<ServiceImpl<typeof ReleaseRequestService>> {
  const r = fx.request;
  const view = () => {
    const built = releaseOrder.indexOf(r.state) > 1 || ["released", "rejected"].includes(r.state);
    return {
      id: r.id,
      appId: "app1",
      environmentId: "env-dev",
      channel: "production",
      state: r.state,
      hold: r.hold ?? "",
      revision: 1n,
      plugins: r.plugins,
      builds: built
        ? r.plugins.map((p) => ({
            pluginKey: p.pluginKey,
            jobId: `job-${p.pluginKey}`,
            state: "succeeded",
            version: 1n,
            bundleSha256: "ab".repeat(16),
            errors: r.errors ?? 0,
            warnings: r.warnings ?? [],
          }))
        : [],
      acknowledged: r.acknowledged ?? [],
      contentHash: built ? releaseHash : "",
      createdBy: r.createdBy ?? "u1",
      createdByName: r.createdByName ?? "Ada",
      releaseSequence: r.state === "released" ? 7n : 0n,
      approvedHash: releaseOrder.indexOf(r.state) > 6 ? releaseHash : "",
    };
  };
  const move = (to: string) => {
    r.state = to;
    return { request: view() };
  };
  return {
    createReleaseRequest: (req) => {
      fx.calls.push(`create:${req.plugins.map((p) => p.pluginKey).join(",")}`);
      return { request: view(), existing: false };
    },
    listReleaseRequests: () => ({ requests: r.state === "released" ? [] : [view()] }),
    getReleaseRequest: () => ({
      request: view(),
      visualDiff: fx.diff && {
        id: "vd1",
        state: fx.diff.state,
        contentHash: releaseHash,
        screens: [{ pluginKey: "home", screen: "main", device: "phone", afterSha256: "aa", changed: true }],
      },
    }),
    advanceReleaseRequest: (req) => {
      if (req.expectedState !== r.state) {
        refused("PLX-8060");
      }
      fx.calls.push(`advance:${r.state}`);
      if (req.step.case === "reports") {
        r.acknowledged = req.step.value.acknowledgedWarnings;
      }
      if (r.state === "diffs" && fx.diff?.state !== "succeeded") {
        refused("PLX-8062");
      }
      return move(releaseOrder[releaseOrder.indexOf(r.state) + 1] ?? "released");
    },
    refreezeReleaseRequest: () => {
      fx.calls.push("refreeze");
      return move("build");
    },
    requestVisualDiff: () => {
      fx.calls.push("diff");
      fx.diff = { state: "succeeded" };
      return { job: { id: "vd1", state: "succeeded", contentHash: releaseHash } };
    },
    requestReleaseApproval: () => {
      fx.calls.push("approval");
      if (fx.diff?.state !== "succeeded") {
        refused("PLX-8062");
      }
      return move(fx.rejectApproval ? "rejected" : "rollout");
    },
    completeReleaseRequest: () => {
      fx.calls.push("complete");
      return move("released");
    },
    cancelReleaseRequest: () => {
      fx.calls.push("cancel");
      return move("cancelled");
    },
  };
}
const releaseOf = (r: ReleasesFixture["releases"][number]) =>
  create(ReleaseSchema, {
    id: `rel-${r.sequence}`,
    sequence: BigInt(r.sequence),
    notes: r.notes ?? "",
    rollbackOf: BigInt(r.rollbackOf ?? 0),
    createdAt: timestampFromMs(1_700_000_000_000),
  });

/** The ReleaseService reads of the Releases area; empty without a fixture. */
function releaseHandlers(data: ReleasesFixture | undefined) {
  const fail = () => {
    if (data?.failing === true) {
      throw new ConnectError("unavailable", Code.Unavailable);
    }
  };
  return {
    listReleases: () => {
      fail();
      return { releases: (data?.releases ?? []).map(releaseOf) };
    },
    getRelease: (req: { sequence: bigint }) => {
      fail();
      const found = data?.releases.find((r) => BigInt(r.sequence) === req.sequence);
      return { release: found === undefined ? undefined : releaseOf(found), versions: [] };
    },
    getChangelog: () => ({ entries: [{ kind: "page", change: "added", pluginKey: "loans", name: "home" }], notes: "" }),
    rollbackRelease: (req: { toSequence: bigint; channelKey: string }) => {
      data?.calls?.push(`rollback:${req.toSequence}:${req.channelKey}`);
      return {};
    },
  };
}

/** The rollout and control RPCs of the Releases area, in memory. */
function releasesServices(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  data: ReleasesFixture,
) {
  const rollout = (r: NonNullable<ReleasesFixture["rollouts"]>[number]) =>
    create(RolloutSchema, {
      id: r.id,
      environmentKey: "development",
      channel: "production",
      candidateSequence: BigInt(r.candidate),
      state: r.state,
      basisPoints: r.basisPoints,
      stageIndex: r.stageIndex ?? 0,
      stages: r.stages.map((basisPoints) => ({ basisPoints })),
      lastCheck: new TextEncoder().encode(r.lastCheck === undefined ? "" : JSON.stringify(r.lastCheck)),
    });
  const find = (id: string) => {
    const found = (data.rollouts ?? []).find((r) => r.id === id);
    return { rollout: found === undefined ? undefined : rollout(found) };
  };
  const act = (name: string, id: string, detail = "") => {
    data.calls?.push(`${name}:${id}${detail}`);
    return find(id);
  };
  service(RolloutService, {
    listRollouts: () => ({ rollouts: (data.rollouts ?? []).map(rollout) }),
    pauseRollout: (req) => act("pause", req.id),
    resumeRollout: (req) => act("resume", req.id),
    advanceRollout: (req) => act("advance", req.id),
    abortRollout: (req) => act("abort", req.id),
    rescheduleRollout: (req) => act("reschedule", req.id, req.clearEndAt ? ":clear" : ":end"),
  });
  const control = () => create(ControlSchema, { killSwitchPlugins: data.killed ?? [] });
  service(ControlService, {
    getControl: () => ({ control: control() }),
    setKillSwitch: (req) => {
      data.calls?.push(`kill:${req.killSwitch?.target}`);
      data.killed = [...(data.killed ?? []), req.killSwitch?.target ?? ""];
      return {};
    },
    clearKillSwitch: (req) => {
      data.calls?.push(`unkill:${req.killSwitch?.target}`);
      data.killed = (data.killed ?? []).filter((k) => k !== req.killSwitch?.target);
      return {};
    },
  });
}

/** The experiment and flag RPCs of the Experiments area, in memory. */
function experimentsServices(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  data: ExperimentsFixture,
) {
  const view = (x: ExperimentsFixture["experiments"][number]) => ({
    id: x.id,
    key: x.key,
    state: x.state,
    winner: x.winner ?? "",
    definitionJson: JSON.stringify(x.definition ?? {}),
  });
  const find = (id: string) => data.experiments.find((x) => x.id === id);
  const guard = () => {
    if (data.failing === true) {
      throw new ConnectError("failing", Code.Internal);
    }
  };
  const mean = (variant: string, value: number) => ({ variant, subjects: 500n, mean: value });
  service(ExperimentService, {
    listExperiments: (req) => {
      guard();
      return { experiments: data.experiments.filter((x) => req.state === 0 || x.state === req.state).map(view) };
    },
    getExperiment: (req) => ({ experiment: view(find(req.id) ?? { id: req.id, key: "", state: 1 }) }),
    createExperiment: (req) => {
      const doc = JSON.parse(new TextDecoder().decode(req.document)) as { key: string };
      data.calls?.push(`create:${doc.key}:${new TextDecoder().decode(req.document)}`);
      return { experiment: view({ id: "exp-new", key: doc.key, state: 1 }) };
    },
    startExperiment: (req) => {
      data.calls?.push(`start:${req.id}`);
      return { experiment: view({ id: req.id, key: "", state: 2 }) };
    },
    stopExperiment: (req) => {
      data.calls?.push(`stop:${req.id}`);
      return { experiment: view({ id: req.id, key: "", state: 3 }) };
    },
    promoteExperiment: (req) => {
      data.calls?.push(`ship:${req.id}:${req.variant}:${req.label}`);
      return { publishJobId: "job-1" };
    },
    getResults: (req) => ({
      experimentId: req.id,
      sampleRatioMismatch: find(req.id)?.key === "mismatch",
      variants: [
        { name: "control", control: true, subjects: 500n, share: 0.5 },
        { name: "bold", subjects: 500n, share: 0.5 },
      ],
      primary: {
        event: "purchase",
        means: [mean("control", 0.1), mean("bold", 0.12)],
        comparisons: [{ variant: "bold", diff: 0.02, lower: 0.01, upper: 0.03, pValue: 0.01, improved: true }],
      },
    }),
  });
  const flag = (f: NonNullable<ExperimentsFixture["flags"]>[number]) => ({
    id: `flag-${f.key}`,
    key: f.key,
    type: f.type,
    defaultJson: f.defaultJson,
    description: f.description ?? "",
  });
  service(FlagService, {
    listFlags: () => ({ flags: (data.flags ?? []).map(flag) }),
    createFlag: (req) => {
      data.calls?.push(`createFlag:${req.key}:${req.spec?.defaultJson}`);
      data.flags = [
        ...(data.flags ?? []),
        { key: req.key, type: req.spec?.type ?? 1, defaultJson: req.spec?.defaultJson ?? "" },
      ];
      return {};
    },
    updateFlag: (req) => {
      data.calls?.push(`updateFlag:${req.key}:${req.spec?.defaultJson}`);
      data.flags = (data.flags ?? []).map((f) =>
        f.key === req.key ? { ...f, defaultJson: req.spec?.defaultJson ?? f.defaultJson } : f,
      );
      return {};
    },
    deleteFlag: (req) => {
      data.calls?.push(`deleteFlag:${req.key}`);
      data.flags = (data.flags ?? []).filter((f) => f.key !== req.key);
      return {};
    },
  });
}

/** The approval and break-glass RPCs of the Approvals area, in memory. */
function approvalsServices(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  data: ApprovalsData,
) {
  const fail = () => {
    if (data.failing === true) {
      throw new ConnectError("unavailable", Code.Unavailable);
    }
  };
  const request = (r: NonNullable<ApprovalsData["requests"]>[number]) =>
    create(ApprovalRequestSchema, {
      id: r.id,
      appId: r.appId,
      status: r.status,
      requestedBy: r.requestedBy,
      summary: r.summary,
      subject: r.subject ?? "release:12",
      resource: r.resource ?? "release",
      environment: "production",
      approvalsRequired: r.approvalsRequired ?? 2,
      policyKeys: ["default-standard-production-release"],
      decisions: (r.decisions ?? []).map((d) => ({ ...d, auditSequence: BigInt(d.auditSequence ?? 0) })),
    });
  const refuse = () => {
    if (data.refusal !== undefined) {
      throw new ConnectError(data.refusal, Code.PermissionDenied);
    }
  };
  service(ApprovalService, {
    listApprovals: (req) => {
      fail();
      const all = (data.requests ?? []).filter((r) => req.appId === "" || r.appId === req.appId);
      return { approvals: all.filter((r) => req.status === "" || r.status === req.status).map(request) };
    },
    getApprovalReview: (req) => {
      fail();
      const found = (data.requests ?? []).find((r) => r.id === req.id);
      if (found === undefined) {
        throw new ConnectError("not found", Code.NotFound);
      }
      return {
        approval: request(found),
        diff: new TextEncoder().encode(found.diff ?? ""),
        risk: { level: found.risk ?? "low", reasons: [] },
        changelog: ["Fix the checkout total"],
        installBase: [{ channel: "stable", releaseSequence: 12n, hostBuild: "1.2.0", devices: 40n }],
      };
    },
    decideApproval: (req) => {
      refuse();
      data.decided?.push({ appId: req.appId, id: req.id, decision: req.decision, comment: req.comment });
      return {};
    },
    listPolicies: () => {
      fail();
      return {
        policies: (data.policies ?? []).map((p) => ({
          key: p.key,
          version: 1,
          document: new TextEncoder().encode(
            JSON.stringify({
              key: p.key,
              name: p.name,
              match: { resources: ["release"] },
              require: { approvers: p.approvers },
            }),
          ),
        })),
      };
    },
    putPolicy: (req) => {
      data.saved?.push(new TextDecoder().decode(req.document));
      return {};
    },
    simulatePolicy: (req) => {
      data.simulated?.push({
        document: new TextDecoder().decode(req.document),
        resource: req.change?.resource ?? "",
        environment: req.change?.environment ?? "",
      });
      return {
        matches: true,
        approvalsRequired: 3,
        requirements: [{ policyKey: "mine", draft: true, approvers: 3, roles: ["reviewer"] }],
      };
    },
  });
  service(BreakGlassService, {
    listBreakGlass: () => {
      fail();
      return {
        invocations: (data.breakGlass ?? []).map((b) => ({
          id: b.id,
          action: b.action,
          subject: b.subject,
          invokedBy: b.invokedBy,
          reviewStatus: b.reviewStatus,
          overdue: b.overdue ?? false,
          environment: "production",
        })),
      };
    },
    reviewBreakGlass: (req) => {
      refuse();
      data.reviewed?.push({ id: req.id, comment: req.comment });
      return {};
    },
  });
}

/** The FunctionAdminService RPCs of the Functions area, in memory. */
function functionsServices(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  data: FunctionsFixture,
) {
  const version = (f: FunctionsFixture["functions"][number]) => ({
    appId: "app1",
    name: f.name,
    version: f.version,
    placement: f.placement ?? "server",
    moduleSha256: `module-${f.name}`,
    abiVersion: 1,
    inputSchema: '{"type":"object"}',
    outputSchema: '{"type":"number"}',
    testsPassed: f.testsPassed ?? true,
    signature: f.signed === false ? new Uint8Array() : new Uint8Array([1]),
    provenance: {
      sourceSha256: `source-${f.name}`,
      toolchain: "rust-1.90",
      builderIdentity: "builder-1",
      dependencies: [{ name: "serde", version: "1.0.0", checksum: "x" }],
    },
  });
  const fail = () => {
    if (data.failing === true) {
      throw new ConnectError("down", Code.Unavailable);
    }
  };
  service(FunctionAdminService, {
    listFunctions: () => {
      fail();
      return { functions: data.functions.map(version) };
    },
    getFunction: (req) => {
      fail();
      const found = data.functions.find((f) => f.name === req.name);
      return { latest: found === undefined ? undefined : version(found), aliases: data.aliases ?? [] };
    },
    listVersions: (req) => ({
      versions: data.functions.filter((f) => f.name === req.name).map(version),
    }),
    setAlias: (req) => {
      data.calls?.push(`${req.alias?.name}:${req.alias?.version}`);
      return { aliases: [req.alias ?? { name: "", version: 0 }] };
    },
    getMetrics: () => ({
      invocations: BigInt(data.metrics?.invocations ?? 0),
      errorsByKind: Object.fromEntries(
        Object.entries(data.metrics?.errorsByKind ?? {}).map(([k, n]) => [k, BigInt(n)]),
      ),
      latencyP95Micros: BigInt(data.metrics?.p95 ?? 0),
    }),
    listLogs: () => ({
      lines: (data.logs ?? []).map((l) => ({ time: { seconds: 1n, nanos: 0 }, version: l.version ?? 1, ...l })),
    }),
    async *runTests() {
      const results = data.tests?.results ?? [];
      for (const r of results) {
        yield { event: { case: "result", value: r } };
      }
      yield {
        event: {
          case: "finished",
          value: {
            passed: results.filter((r) => r.passed).length,
            failed: results.filter((r) => !r.passed).length,
            diagnostics: new Uint8Array(),
          },
        },
      };
    },
  });
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import {
  type ApplyChangeSetRequest,
  AppService,
  DevDeviceSchema,
  type DevEvent,
  DevSessionService,
  type DevSessionServiceSendCommandRequest,
  DocumentSchema,
  DocumentService,
  FederationService,
  FunctionAdminService,
  GetCurrentUserResponseSchema,
  GovernanceService,
  IdentityService,
  InboxItemSchema,
  LanguageServerService,
  LockSchema,
  MemberSchema,
  NotificationService,
  OrganizationSchema,
  OrgService,
  PluginSchema,
  PluginService,
  PluginVersionSchema,
  PreferencesSchema,
  PreferencesService,
  ReleaseService,
  UserSchema,
} from "@plux/api";
import { applyPatch, type JsonPatchOp, type JsonValue } from "./features/editor/json-patch.ts";

export const orgA = "0192f000-0000-7000-8000-00000000000a";
export const orgB = "0192f000-0000-7000-8000-00000000000b";

export interface UpstreamData {
  /** The dev session (DEV-002, DEV-011, DEV-020): paired devices, the pairing token, the event stream and what Studio sent. */
  devSession?: {
    devices: { id: string; name: string; connected: boolean }[];
    pairing?: { uri: string; code: string; expiresInSeconds: number };
    events?: DevEvent[];
    /** Collects every SendCommand. */
    commands?: DevSessionServiceSendCommandRequest[];
    /** Collects the device filter of every ExportTrace. */
    exported?: string[][];
  };
  signedIn?: boolean;
  permissions?: string[];
  installationAdmin?: boolean;
  csrfToken?: string;
  memberships?: string[];
  stored?: { theme: string; language: string; density: string };
  explained?: { permission: string }[];
  inbox?: { id: string; eventType: string; title: string; read: boolean }[];
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
  updates?: { paths: string[]; theme: string; language: string; density: string }[];
}

/** An in-memory upstream for the shell: the same RPCs the BFF forwards. */
export function createUpstream(state: UpstreamData) {
  // Documents autosave changed, by path; the fixtures themselves stay as given.
  const patched = new Map<string, { kind: string; path: string; json: unknown }>();
  const stored = state.stored ?? { theme: "", language: "", density: "" };
  return createRouterTransport(({ service }) => {
    service(IdentityService, {
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
      explainAccess: (req) => {
        state.explained?.push({ permission: req.permission });
        return { decision: "deny" };
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
      listEnvironments: () => ({ environments: [{ id: "env-prod", key: "production", production: true }] }),
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
      listTrash: () => ({ items: state.trash ?? [] }),
      restoreFromTrash: (req) => {
        state.restored?.push(req.id);
        return {};
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
    service(ReleaseService, {
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
      getDocument: (req) => {
        const found = (state.snapshots ?? []).flatMap((x) => x.documents).find((d) => d.path === req.path);
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
    service(FederationService, { listSignInProviders: () => ({ providers: [] }) });
  });
}

type DevSessionData = NonNullable<UpstreamData["devSession"]>;

/** The dev session service in memory: pairing, devices, commands, a fixed event stream and the trace export. */
function devSessionService(
  service: Parameters<Parameters<typeof createRouterTransport>[0]>[0]["service"],
  dev: DevSessionData,
) {
  service(DevSessionService, {
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

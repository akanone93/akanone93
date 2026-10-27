// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import {
  type ApplyChangeSetRequest,
  AppService,
  DocumentSchema,
  DocumentService,
  FederationService,
  GetCurrentUserResponseSchema,
  GovernanceService,
  IdentityService,
  InboxItemSchema,
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
  summaries?: { id: string; key: string; name: string; latestVersion?: number }[];
  /** Collects plugin RPCs the app canvas sends, as `delete:<id>` and `archive:<id>`. */
  pluginCalls?: string[];
  /** The app's trash, and the ids restored from it. */
  trash?: { id: string; kind: string; name: string }[];
  restored?: string[];
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
      getSnapshot: (req) => ({
        documents: (req.id === "snap-1" ? (state.published ?? []) : [])
          .filter((d) => req.paths.includes(d.path))
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
    service(FederationService, { listSignInProviders: () => ({ providers: [] }) });
  });
}
